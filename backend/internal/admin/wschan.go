// wschan.go 最小 RFC6455 WebSocket 服务端实现（零第三方依赖）。
//
// 范围刻意收窄到「服务端单向推送 + 控制帧」：
//   - 写：文本帧（不掩码，服务端规范）、ping、close（1000/1013）；
//   - 读：只消费控制帧（close → 语义断开；ping → pong 回显），
//     数据帧读出即弃（管理端实时流是只读订阅，客户端不发业务数据）；
//   - 不协商 permessage-deflate（不声明即不压缩）、不做分片重组。
//
// 浏览器对帧边界的兼容性由规范保证：单帧完整文本、≤64KiB payload。

package admin

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
)

// wsGUID RFC6455 §1.3 固定握手 GUID。
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsAcceptKey 计算 Sec-WebSocket-Accept。
func wsAcceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

const (
	wsOpText  = 0x1
	wsOpClose = 0x8
	wsOpPing  = 0x9
	wsOpPong  = 0xA
)

// wsWriteFrame 写一个完整服务端帧（fin=1，不掩码）并 Flush。
// w 必须是 http.Hijacker 返回的 *bufio.ReadWriter：其 Write 缓冲在内部
// bufio.Writer，Flush 直接落 TCP —— 不要再在外层包一层 bufio.Writer，
// 否则 Flush 只会写进 rw 自身的缓冲（实测 101 握手因此永远到不了客户端）。
func wsWriteFrame(w *bufio.ReadWriter, opcode byte, payload []byte) error {
	var hdr [10]byte
	hdr[0] = 0x80 | opcode
	n := len(payload)
	switch {
	case n <= 125:
		hdr[1] = byte(n)
		if _, err := w.Write(hdr[:2]); err != nil {
			return err
		}
	case n <= 0xFFFF:
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:4], uint16(n))
		if _, err := w.Write(hdr[:4]); err != nil {
			return err
		}
	default:
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:10], uint64(n))
		if _, err := w.Write(hdr[:10]); err != nil {
			return err
		}
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}

// wsWriteCloseFrame 发送关闭帧（code=1000 正常关闭 或 1013 超载）。
func wsWriteCloseFrame(w *bufio.ReadWriter, code uint16) error {
	var payload [2]byte
	binary.BigEndian.PutUint16(payload[:], code)
	return wsWriteFrame(w, wsOpClose, payload[:])
}

// wsReadClientFrame 读一个客户端帧并解掩码；返回 opcode 与 payload。
// 数据帧 payload 也会完整读出（保持帧流对齐），调用方按需丢弃。
// 恶意超长帧（>1MiB）直接拒绝。
func wsReadClientFrame(r *bufio.Reader) (byte, []byte, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := b0 & 0x0F
	masked := b1&0x80 != 0
	n := int64(b1 & 0x7F)
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		n = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		n = int64(binary.BigEndian.Uint64(ext[:]))
	}
	if n > 1<<20 {
		return 0, nil, fmt.Errorf("frame too large: %d", n)
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= key[i%4]
		}
	}
	return opcode, payload, nil
}
