package server

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/llm"
)

// wsGUID is the magic string specified by RFC 6455 for the WebSocket handshake.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// --- WebSocket frame opcodes ---
const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA
)

// wsConn wraps the hijacked TCP connection together with its buffered reader
// so that any bytes the HTTP server may have already buffered are not lost.
type wsConn struct {
	conn net.Conn
	bufw *bufio.ReadWriter
}

func (c *wsConn) Close() error { return c.conn.Close() }

// WSMessage is the JSON envelope exchanged over /ws/chat.
type WSMessage struct {
	Type       string      `json:"type"`                // "chat" | "ping" | "pong" | "error"
	Role       string      `json:"role"`                // LLM role: "worker" | "orchestrator" | ...
	Content    string      `json:"content"`             // user message or assistant reply
	ProviderID string      `json:"provider_id,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

// handleWSChat handles GET /ws/chat — upgrades to WebSocket, then runs a
// chat loop using the configured LLM router.
func (s *Server) handleWSChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, api.ErrCodeInvalidInput, "method not allowed")
		return
	}

	wsc, err := upgradeWebSocket(w, r)
	if err != nil {
		log.Printf("ws: upgrade failed: %v", err)
		return
	}
	defer wsc.Close()

	log.Printf("ws: client connected from %s", r.RemoteAddr)

	// Per-connection conversation history.
	var (
		mu       sync.Mutex
		messages []llm.Message
	)

	for {
		msg, err := wsReadMessage(wsc)
		if err != nil {
			if err != io.EOF {
				log.Printf("ws: read error: %v", err)
			}
			return
		}

		var env WSMessage
		if err := json.Unmarshal(msg, &env); err != nil {
			_ = wsSendJSON(wsc, WSMessage{Type: "error", Content: "invalid JSON"})
			continue
		}

		switch env.Type {
		case "ping":
			_ = wsSendJSON(wsc, WSMessage{Type: "pong"})

		case "chat":
			if env.Content == "" {
				_ = wsSendJSON(wsc, WSMessage{Type: "error", Content: "content is required"})
				continue
			}

			mu.Lock()
			messages = append(messages, llm.Message{Role: "user", Content: env.Content})
			snap := make([]llm.Message, len(messages))
			copy(snap, messages)
			mu.Unlock()

			// Route to LLM in a goroutine so the read loop continues.
			go func(history []llm.Message, role, providerID string) {
				reply, err := s.chatWithLLM(r.Context(), role, providerID, history)
				if err != nil {
					_ = wsSendJSON(wsc, WSMessage{Type: "error", Content: err.Error()})
					return
				}
				mu.Lock()
				messages = append(messages, llm.Message{Role: "assistant", Content: reply})
				mu.Unlock()
				_ = wsSendJSON(wsc, WSMessage{Type: "chat", Role: "assistant", Content: reply})
			}(snap, env.Role, env.ProviderID)

		default:
			_ = wsSendJSON(wsc, WSMessage{Type: "error", Content: fmt.Sprintf("unknown type %q", env.Type)})
		}
	}
}

// chatWithLLM resolves the role to a provider and runs a single chat turn.
func (s *Server) chatWithLLM(ctx context.Context, role, providerID string, messages []llm.Message) (string, error) {
	if s.router == nil {
		return "", fmt.Errorf("router not configured")
	}

	// If a specific provider ID was requested, look it up directly.
	if providerID != "" {
		p, err := s.db.GetProvider(ctx, providerID)
		if err == nil {
			prov, err := s.llmReg.Get(p.Name)
			if err == nil {
				resp, err := prov.Chat(ctx, llm.ChatRequest{
					Model:     p.ModelName,
					Messages:  messages,
					MaxTokens: 4096,
				})
				if err != nil {
					return "", fmt.Errorf("llm error: %w", err)
				}
				return resp.Content, nil
			}
		}
		// Fall through to role-based routing if provider lookup fails.
	}

	targetRole := role
	if targetRole == "" {
		targetRole = "orchestrator" // sensible default for chat
	}

	result, err := s.router.RouteByRole(targetRole)
	if err != nil {
		// Try orchestrator as fallback.
		result, err = s.router.RouteByRole("orchestrator")
		if err != nil {
			return "", fmt.Errorf("no provider for role %q: %w", targetRole, err)
		}
	}
	if result.Provider == nil {
		return "", fmt.Errorf("provider for role %q is nil", targetRole)
	}

	resp, err := result.Provider.Chat(ctx, llm.ChatRequest{
		Model:     result.Model,
		Messages:  messages,
		MaxTokens: 4096,
	})
	if err != nil {
		return "", fmt.Errorf("llm error: %w", err)
	}
	return resp.Content, nil
}

// ---------------------------------------------------------------------------
// Minimal RFC 6455 WebSocket implementation (no external deps)
// ---------------------------------------------------------------------------

// upgradeWebSocket performs the HTTP → WebSocket upgrade handshake.
// Returns a wsConn that wraps the hijacked connection + its buffered I/O.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return nil, fmt.Errorf("not a websocket request")
	}
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-Websocket-Key", http.StatusBadRequest)
		return nil, fmt.Errorf("missing Sec-Websocket-Key")
	}

	// Compute RFC 6455 accept key.
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return nil, fmt.Errorf("hijack not supported")
	}
	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack: %w", err)
	}

	// Send 101 Switching Protocols response.
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := io.WriteString(bufrw, resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write handshake: %w", err)
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("flush handshake: %w", err)
	}
	return &wsConn{conn: conn, bufw: bufrw}, nil
}

// wsReadMessage reads a single complete WebSocket text/binary frame.
// It handles client masking, ping/pong, and close frames.
func wsReadMessage(c *wsConn) ([]byte, error) {
	r := c.bufw.Reader
	for {
		// Read first two bytes (FIN+opcode, MASK+payload length).
		header := make([]byte, 2)
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, err
		}

		opcode := header[0] & 0x0F
		masked := (header[1] & 0x80) != 0
		payloadLen := int64(header[1] & 0x7F)

		switch {
		case payloadLen == 126:
			var ext uint16
			if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
				return nil, err
			}
			payloadLen = int64(ext)
		case payloadLen == 127:
			if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
				return nil, err
			}
		}

		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(r, mask[:]); err != nil {
				return nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}

		switch opcode {
		case wsOpText, wsOpBinary:
			return payload, nil
		case wsOpClose:
			_ = wsSendFrame(c, wsOpClose, nil)
			return nil, io.EOF
		case wsOpPing:
			_ = wsSendFrame(c, wsOpPong, payload)
		case wsOpPong:
			// ignore
		case wsOpContinuation:
			// fragmentation not supported — return payload of first fragment
			return payload, nil
		}
	}
}

// wsSendJSON encodes v as JSON and sends it as a text WebSocket frame.
func wsSendJSON(c *wsConn, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return wsSendFrame(c, wsOpText, b)
}

// wsSendFrame writes a single unmasked WebSocket frame to the connection.
// Server → client frames must NOT be masked per RFC 6455.
func wsSendFrame(c *wsConn, opcode byte, payload []byte) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	frame := make([]byte, 0, 10+len(payload))
	frame = append(frame, 0x80|opcode) // FIN + opcode

	l := len(payload)
	switch {
	case l <= 125:
		frame = append(frame, byte(l))
	case l <= 65535:
		frame = append(frame, 126, byte(l>>8), byte(l))
	default:
		frame = append(frame, 127)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(l))
		frame = append(frame, b...)
	}
	frame = append(frame, payload...)
	_, err := c.conn.Write(frame)
	return err
}
