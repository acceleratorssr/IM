package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

/*
+-------------------+---------------+------------+-----------+--------------+------------+-------------+-----------------+
|    packSize       |  headerSize   |  verSize   | statusSize| serviceSize  |  cmdSize   |  seqSize    |     Body        |
|      (4 bytes)    |   (2 bytes)   | (1 byte)   | (1 byte)  |  (2 bytes)   | (2 bytes)  |  (4 bytes)  | (N bytes, 		 |
|                   |               |            |           |              |            |             | N = packSize -  |
|                   |       ?       |            |           |              |            |             | headerSize)     |
+-------------------+---------------+------------+-----------+--------------+------------+-------------+-----------------+
0                   4               6           7           8             10          12            16                ...
*/

const (
	maxBodySize = 1 << 12

	packSize      = 4
	headerSize    = 2 // 记录接下来是多少字节（rawHeaderSize）的头部信息。
	verSize       = 1
	statusSize    = 1
	serviceIdSize = 2
	cmdSize       = 2
	seqSize       = 4

	rawHeaderSize = verSize + statusSize + serviceIdSize + cmdSize + seqSize

	maxPackSize = maxBodySize + rawHeaderSize + headerSize + packSize

	// offset
	headerOffset    = 0
	verOffset       = headerOffset + headerSize
	statusOffset    = verOffset + verSize
	serviceIdOffset = statusOffset + statusSize
	cmdOffset       = serviceIdOffset + serviceIdSize
	seqOffset       = cmdOffset + cmdSize
	bodyOffset      = seqOffset + seqSize
)

var (
	ErrRawPackLen       = errors.New("packet length error")
	ErrRawHeaderLen     = errors.New("header length error")
	ErrInvalidBody      = errors.New("invalid packet, insufficient header data")
	ErrPackSizeTooSmall = errors.New("packet size too small")
)

type Header struct {
	Version   uint8
	Status    uint8
	ServiceId uint16
	Cmd       uint16
	Seq       uint32
}

type Message struct {
	Header
	Body []byte
}

func (m *Message) Format() string {
	return fmt.Sprintf("Version:%d, Status:%d, ServiceId:%d, Cmd:%d, Seq:%d, Body:%s",
		m.Version, m.Status, m.ServiceId, m.Cmd, m.Seq, string(m.Body))
}

type Protocol interface {
	NewCodec(conn net.Conn) Codec
}

type Codec interface {
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Receive() (*Message, error)
	Send(Message) error
	Close() error
}

type IMProtocol struct{}

func NewIMProtocol() Protocol {
	return &IMProtocol{}
}

func (p *IMProtocol) NewCodec(conn net.Conn) Codec {
	return &imCodec{conn: conn}
}

type imCodec struct {
	conn net.Conn
}

func (c *imCodec) readPackSize() (uint32, error) {
	return c.readUint32BE()
}

func (c *imCodec) readUint32BE() (uint32, error) {
	b := make([]byte, packSize)
	_, err := io.ReadFull(c.conn, b)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (c *imCodec) readPacket(msgSize uint32) ([]byte, error) {
	b := make([]byte, msgSize)
	_, err := io.ReadFull(c.conn, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (c *imCodec) Receive() (*Message, error) {
	packLen, err := c.readPackSize()
	if err != nil {
		return nil, err
	}

	if packLen > maxPackSize {
		return nil, ErrRawPackLen
	}
	if packLen < headerSize+rawHeaderSize {
		return nil, ErrPackSizeTooSmall
	}

	buf, err := c.readPacket(packLen)
	if err != nil {
		return nil, err
	}
	if len(buf) < bodyOffset {
		return nil, ErrInvalidBody
	}

	var msg Message

	headerLen := binary.BigEndian.Uint16(buf[headerOffset:verOffset])
	msg.Version = buf[verOffset]
	msg.Status = buf[statusOffset]
	msg.ServiceId = binary.BigEndian.Uint16(buf[serviceIdOffset:cmdOffset])
	msg.Cmd = binary.BigEndian.Uint16(buf[cmdOffset:seqOffset])
	msg.Seq = binary.BigEndian.Uint32(buf[seqOffset:bodyOffset])

	if headerLen != rawHeaderSize {
		return nil, ErrRawHeaderLen
	}

	if packLen > uint32(headerLen) {
		msg.Body = buf[bodyOffset:packLen]
	}

	logx.Infof("receive msg:%+v", msg)
	return &msg, nil
}

func (c *imCodec) Send(msg Message) error {
	packLen := headerSize + rawHeaderSize + len(msg.Body)
	packLenBuf := make([]byte, packSize)
	binary.BigEndian.PutUint32(packLenBuf[:packSize], uint32(packLen))

	buf := make([]byte, packLen)
	// header
	binary.BigEndian.PutUint16(buf[headerOffset:], uint16(rawHeaderSize))
	buf[verOffset] = msg.Version
	buf[statusOffset] = msg.Status
	binary.BigEndian.PutUint16(buf[serviceIdOffset:], msg.ServiceId)
	binary.BigEndian.PutUint16(buf[cmdOffset:], msg.Cmd)
	binary.BigEndian.PutUint32(buf[seqOffset:], msg.Seq)

	// body
	copy(buf[headerSize+rawHeaderSize:], msg.Body)
	allBuf := append(packLenBuf, buf...)
	n, err := c.conn.Write(allBuf)
	if err != nil {
		return err
	}
	if n != len(allBuf) {
		return fmt.Errorf("n:%d, len(buf):%d", n, len(buf))
	}
	return nil
}

func (c *imCodec) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *imCodec) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *imCodec) Close() error {
	return c.conn.Close()
}
