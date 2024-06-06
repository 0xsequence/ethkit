// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsReadBuffer       = 1024
	wsWriteBuffer      = 1024
	wsPingInterval     = 30 * time.Second
	wsPingWriteTimeout = 5 * time.Second
	wsPongTimeout      = 30 * time.Second
	wsDefaultReadLimit = 32 * 1024 * 1024
)

var wsBufferPool = new(sync.Pool)

type wsHandshakeError struct {
	err    error
	status string
}

func (e wsHandshakeError) Error() string {
	s := e.err.Error()
	if e.status != "" {
		s += " (HTTP status " + e.status + ")"
	}
	return s
}

func (e wsHandshakeError) Unwrap() error {
	return e.err
}

// DialWebsocketWithDialer creates a new RPC client using WebSocket.
//
// The context is used for the initial connection establishment. It does not
// affect subsequent interactions with the client.
//
// Deprecated: use DialOptions and the WithWebsocketDialer option.
func DialWebsocketWithDialer(ctx context.Context, endpoint, origin string, dialer websocket.Dialer) (*Client, error) {
	cfg := new(clientConfig)
	cfg.wsDialer = &dialer
	if origin != "" {
		cfg.setHeader("origin", origin)
	}
	connect, err := newClientTransportWS(endpoint, cfg)
	if err != nil {
		return nil, err
	}
	return newClient(ctx, cfg, connect)
}

// DialWebsocket creates a new RPC client that communicates with a JSON-RPC server
// that is listening on the given endpoint.
//
// The context is used for the initial connection establishment. It does not
// affect subsequent interactions with the client.
func DialWebsocket(ctx context.Context, endpoint, origin string) (*Client, error) {
	cfg := new(clientConfig)
	if origin != "" {
		cfg.setHeader("origin", origin)
	}
	connect, err := newClientTransportWS(endpoint, cfg)
	if err != nil {
		return nil, err
	}
	return newClient(ctx, cfg, connect)
}

func newClientTransportWS(endpoint string, cfg *clientConfig) (reconnectFunc, error) {
	dialer := cfg.wsDialer
	if dialer == nil {
		dialer = &websocket.Dialer{
			ReadBufferSize:  wsReadBuffer,
			WriteBufferSize: wsWriteBuffer,
			WriteBufferPool: wsBufferPool,
			Proxy:           http.ProxyFromEnvironment,
		}
	}

	dialURL, header, err := wsClientHeaders(endpoint, "")
	if err != nil {
		return nil, err
	}
	for key, values := range cfg.httpHeaders {
		header[key] = values
	}

	connect := func(ctx context.Context) (ServerCodec, error) {
		header := header.Clone()
		if cfg.httpAuth != nil {
			if err := cfg.httpAuth(header); err != nil {
				return nil, err
			}
		}
		conn, resp, err := dialer.DialContext(ctx, dialURL, header)
		if err != nil {
			hErr := wsHandshakeError{err: err}
			if resp != nil {
				hErr.status = resp.Status
			}
			return nil, hErr
		}
		messageSizeLimit := int64(wsDefaultReadLimit)
		if cfg.wsMessageSizeLimit != nil && *cfg.wsMessageSizeLimit >= 0 {
			messageSizeLimit = *cfg.wsMessageSizeLimit
		}
		return newWebsocketCodec(conn, dialURL, header, messageSizeLimit), nil
	}
	return connect, nil
}

func wsClientHeaders(endpoint, origin string) (string, http.Header, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return endpoint, nil, err
	}
	header := make(http.Header)
	if origin != "" {
		header.Add("origin", origin)
	}
	if endpointURL.User != nil {
		b64auth := base64.StdEncoding.EncodeToString([]byte(endpointURL.User.String()))
		header.Add("authorization", "Basic "+b64auth)
		endpointURL.User = nil
	}
	return endpointURL.String(), header, nil
}

type websocketCodec struct {
	*jsonCodec
	conn *websocket.Conn
	info PeerInfo

	wg           sync.WaitGroup
	pingReset    chan struct{}
	pongReceived chan struct{}
}

func newWebsocketCodec(conn *websocket.Conn, host string, req http.Header, readLimit int64) ServerCodec {
	conn.SetReadLimit(readLimit)
	encode := func(v interface{}, isErrorResponse bool) error {
		return conn.WriteJSON(v)
	}
	wc := &websocketCodec{
		jsonCodec:    NewFuncCodec(conn, encode, conn.ReadJSON).(*jsonCodec),
		conn:         conn,
		pingReset:    make(chan struct{}, 1),
		pongReceived: make(chan struct{}),
		info: PeerInfo{
			Transport:  "ws",
			RemoteAddr: conn.RemoteAddr().String(),
		},
	}
	// Fill in connection details.
	wc.info.HTTP.Host = host
	wc.info.HTTP.Origin = req.Get("Origin")
	wc.info.HTTP.UserAgent = req.Get("User-Agent")
	// Start pinger.
	conn.SetPongHandler(func(appData string) error {
		select {
		case wc.pongReceived <- struct{}{}:
		case <-wc.closed():
		}
		return nil
	})
	wc.wg.Add(1)
	go wc.pingLoop()
	return wc
}

func (wc *websocketCodec) close() {
	wc.jsonCodec.close()
	wc.wg.Wait()
}

func (wc *websocketCodec) peerInfo() PeerInfo {
	return wc.info
}

func (wc *websocketCodec) writeJSON(ctx context.Context, v interface{}, isError bool) error {
	err := wc.jsonCodec.writeJSON(ctx, v, isError)
	if err == nil {
		// Notify pingLoop to delay the next idle ping.
		select {
		case wc.pingReset <- struct{}{}:
		default:
		}
	}
	return err
}

// pingLoop sends periodic ping frames when the connection is idle.
func (wc *websocketCodec) pingLoop() {
	var pingTimer = time.NewTimer(wsPingInterval)
	defer wc.wg.Done()
	defer pingTimer.Stop()

	for {
		select {
		case <-wc.closed():
			return

		case <-wc.pingReset:
			if !pingTimer.Stop() {
				<-pingTimer.C
			}
			pingTimer.Reset(wsPingInterval)

		case <-pingTimer.C:
			wc.jsonCodec.encMu.Lock()
			wc.conn.SetWriteDeadline(time.Now().Add(wsPingWriteTimeout))
			wc.conn.WriteMessage(websocket.PingMessage, nil)
			wc.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
			wc.jsonCodec.encMu.Unlock()
			pingTimer.Reset(wsPingInterval)

		case <-wc.pongReceived:
			wc.conn.SetReadDeadline(time.Time{})
		}
	}
}
