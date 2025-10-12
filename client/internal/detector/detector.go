package detector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Protocol represents a detected network protocol.
type Protocol string

const (
	ProtocolUnknown   Protocol = "UNKNOWN"
	ProtocolTCP       Protocol = "TCP"
	ProtocolUDP       Protocol = "UDP"
	ProtocolHTTP      Protocol = "HTTP"
	ProtocolHTTPS     Protocol = "HTTPS"
	ProtocolTLS       Protocol = "TLS"
	ProtocolSSH       Protocol = "SSH"
	ProtocolSOCKS4    Protocol = "SOCKS4"
	ProtocolSOCKS5    Protocol = "SOCKS5"
	ProtocolSMTP      Protocol = "SMTP"
	ProtocolPOP3      Protocol = "POP3"
	ProtocolIMAP      Protocol = "IMAP"
	ProtocolFTP       Protocol = "FTP"
	ProtocolDNS       Protocol = "DNS"
	ProtocolRDP       Protocol = "RDP"
	ProtocolVNC       Protocol = "VNC"
	ProtocolMQTT      Protocol = "MQTT"
	ProtocolAMQP      Protocol = "AMQP"
	ProtocolPostgres  Protocol = "POSTGRES"
	ProtocolMySQL     Protocol = "MYSQL"
	ProtocolRedis     Protocol = "REDIS"
	ProtocolMongoDB   Protocol = "MONGODB"
	ProtocolMemcached Protocol = "MEMCACHED"
	ProtocolSNMP      Protocol = "SNMP"
	ProtocolNTP       Protocol = "NTP"
	ProtocolLDAP      Protocol = "LDAP"
	ProtocolKerberos  Protocol = "KERBEROS"
	ProtocolSMB       Protocol = "SMB"
	ProtocolRPC       Protocol = "RPC"
	ProtocolSIP       Protocol = "SIP"
	ProtocolRTSP      Protocol = "RTSP"
	ProtocolRTMP      Protocol = "RTMP"
	ProtocolWebsocket Protocol = "WEBSOCKET"
	ProtocolQUIC      Protocol = "QUIC"
	ProtocolKCP       Protocol = "KCP"
	ProtocolWireGuard Protocol = "WIREGUARD"
	ProtocolOpenVPN   Protocol = "OPENVPN"
	ProtocolIKEv2     Protocol = "IKEV2"
	ProtocolGRE       Protocol = "GRE"
	ProtocolICMP      Protocol = "ICMP"
)

// DetectionResult holds the result of a protocol detection.
type DetectionResult struct {
	Protocol   Protocol
	Confidence float64 // 0.0 to 1.0
	Buffer     []byte  // The buffer read during detection
}

// DetectorConfig configures the protocol detector.
type DetectorConfig struct {
	BufferSize  int           // Max bytes to buffer for detection
	Timeout     time.Duration // Max time to wait for enough data
	MaxAttempts int           // Max attempts to detect
}

// DefaultDetectorConfig provides a default configuration.
func DefaultDetectorConfig() *DetectorConfig {
	return &DetectorConfig{
		BufferSize:  4096,
		Timeout:     2 * time.Second,
		MaxAttempts: 3,
	}
}

// Detector performs protocol detection on a net.Conn.
type Detector struct {
	config *DetectorConfig
	logger *zap.Logger
}

// NewDetector creates a new Detector instance.
func NewDetector(config *DetectorConfig) *Detector {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	return &Detector{
		config: config,
		logger: zap.L().Named("protocol-detector"),
	}
}

// DetectProtocol attempts to detect the protocol of the given connection.
// It reads from the connection, buffers the data, and then tries to identify the protocol.
// The buffered data is returned in the DetectionResult for further processing.
func (d *Detector) DetectProtocol(conn net.Conn) (*DetectionResult, error) {
	// Use a buffered reader to peek at the data without consuming it permanently
	// The buffered reader will wrap the original connection
	reader := bufio.NewReaderSize(conn, d.config.BufferSize)

	// Create a context with timeout for detection
	ctx, cancel := context.WithTimeout(context.Background(), d.config.Timeout)
	defer cancel()

	var buffer bytes.Buffer

	// Try multiple attempts to read enough data for detection
	for i := 0; i < d.config.MaxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("protocol detection timed out after %d attempts: %w", i+1, ctx.Err())
		default:
			// Peek at the available data
			peeked, peekErr := reader.Peek(reader.Buffered())
			if peekErr != nil && peekErr != io.EOF {
				d.logger.Debug("Peek error during detection", zap.Error(peekErr))
				// If it's a temporary error, try again
				if netErr, ok := peekErr.(net.Error); ok && netErr.Temporary() {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return nil, fmt.Errorf("failed to peek data: %w", peekErr)
			}

			// Write peeked data to our buffer for analysis
			if len(peeked) > 0 {
				buffer.Write(peeked)
			}

			// Try to detect with current buffer
			result := d.analyzeBuffer(buffer.Bytes())
			if result.Protocol != ProtocolUnknown {
				// If a protocol is detected, return it along with the buffered data
				return &DetectionResult{
					Protocol:   result.Protocol,
					Confidence: result.Confidence,
					Buffer:     buffer.Bytes(),
				}, nil
			}

			// If not enough data, try to read more
			if buffer.Len() < d.config.BufferSize {
				// Attempt to read more data into the internal buffer of bufio.Reader
				// This will block until data is available or timeout
				conn.SetReadDeadline(time.Now().Add(d.config.Timeout / time.Duration(d.config.MaxAttempts)))
				_, readErr := reader.ReadByte()   // Read one byte to trigger internal buffer fill
				conn.SetReadDeadline(time.Time{}) // Clear deadline
				if readErr != nil {
					if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
						d.logger.Debug("Read timeout during detection attempt", zap.Int("attempt", i+1))
						// If timeout, and still no protocol, continue to next attempt or return unknown
						continue
					}
					if readErr == io.EOF {
						d.logger.Debug("EOF reached during detection", zap.Int("attempt", i+1))
						break // No more data to read
					}
					return nil, fmt.Errorf("failed to read more data for detection: %w", readErr)
				}
				// If a byte was read, it's now in the internal buffer, so we can peek again in the next iteration
			} else {
				// Buffer is full, but still unknown. Break and return unknown.
				break
			}
		}
	}

	// If no protocol was confidently detected after all attempts, return UNKNOWN
	return &DetectionResult{
		Protocol:   ProtocolUnknown,
		Confidence: 0.0,
		Buffer:     buffer.Bytes(),
	}, nil
}

// analyzeBuffer performs the actual protocol identification based on the buffered data.
func (d *Detector) analyzeBuffer(data []byte) *DetectionResult {
	if len(data) == 0 {
		return &DetectionResult{Protocol: ProtocolUnknown, Confidence: 0.0}
	}

	// HTTP/HTTPS detection (starts with HTTP methods or TLS handshake)
	if len(data) >= 4 {
		if bytes.HasPrefix(data, []byte("GET ")) ||
			bytes.HasPrefix(data, []byte("POST ")) ||
			bytes.HasPrefix(data, []byte("PUT ")) ||
			bytes.HasPrefix(data, []byte("HEAD ")) ||
			bytes.HasPrefix(data, []byte("DELETE ")) ||
			bytes.HasPrefix(data, []byte("OPTIONS ")) ||
			bytes.HasPrefix(data, []byte("CONNECT ")) ||
			bytes.HasPrefix(data, []byte("TRACE ")) {
			return &DetectionResult{Protocol: ProtocolHTTP, Confidence: 0.9}
		}
		// TLS handshake (ClientHello starts with 0x16, then TLS version, then length)
		if data[0] == 0x16 && (data[1] == 0x03 && (data[2] >= 0x01 && data[2] <= 0x04)) {
			return &DetectionResult{Protocol: ProtocolTLS, Confidence: 0.95}
		}
	}

	// SSH detection (starts with "SSH-")
	if bytes.HasPrefix(data, []byte("SSH-")) {
		return &DetectionResult{Protocol: ProtocolSSH, Confidence: 0.9}
	}

	// SOCKS5 detection (starts with 0x05)
	if len(data) >= 1 && data[0] == 0x05 {
		return &DetectionResult{Protocol: ProtocolSOCKS5, Confidence: 0.8}
	}

	// SOCKS4 detection (starts with 0x04)
	if len(data) >= 1 && data[0] == 0x04 {
		return &DetectionResult{Protocol: ProtocolSOCKS4, Confidence: 0.8}
	}

	// SMTP detection (starts with "220 " or "HELO" etc.)
	if len(data) >= 4 && (bytes.HasPrefix(data, []byte("220 ")) || bytes.Contains(data, []byte("HELO")) || bytes.Contains(data, []byte("EHLO"))) {
		return &DetectionResult{Protocol: ProtocolSMTP, Confidence: 0.7}
	}

	// POP3 detection (starts with "+OK")
	if bytes.HasPrefix(data, []byte("+OK")) {
		return &DetectionResult{Protocol: ProtocolPOP3, Confidence: 0.7}
	}

	// IMAP detection (starts with "* OK")
	if bytes.HasPrefix(data, []byte("* OK")) {
		return &DetectionResult{Protocol: ProtocolIMAP, Confidence: 0.7}
	}

	// FTP detection (starts with "220 " or "FTP")
	if len(data) >= 4 && (bytes.HasPrefix(data, []byte("220 ")) || bytes.Contains(data, []byte("FTP"))) {
		return &DetectionResult{Protocol: ProtocolFTP, Confidence: 0.6}
	}

	// DNS detection (UDP, but can be seen in TCP if it's a zone transfer or long query)
	// DNS query ID is first 2 bytes, then flags. Standard query has 0x0100 flags.
	if len(data) >= 4 && (data[2]&0x80 == 0) { // Check if it's a query (QR bit is 0)
		// This is a very weak indicator for TCP DNS, more reliable for UDP.
		// For TCP, it's often preceded by a 2-byte length field.
		// We'll assume it's TCP if we're here and haven't detected anything else.
		return &DetectionResult{Protocol: ProtocolDNS, Confidence: 0.5}
	}

	// WebSocket detection (HTTP Upgrade header)
	if strings.Contains(string(data), "Upgrade: websocket") {
		return &DetectionResult{Protocol: ProtocolWebsocket, Confidence: 0.85}
	}

	// Default to TCP if nothing else is detected and there's data
	if len(data) > 0 {
		return &DetectionResult{Protocol: ProtocolTCP, Confidence: 0.1} // Low confidence default
	}

	return &DetectionResult{Protocol: ProtocolUnknown, Confidence: 0.0}
}
