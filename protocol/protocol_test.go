package protocol

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

// Helper function to create a Message for testing
func createTestMessage() Message {
	return Message{
		Header: Header{
			Version:   1,
			Status:    0,
			ServiceId: 1000,
			Cmd:       200,
			Seq:       123456789,
		},
		Body: []byte("Hello, World!"),
	}
}

// Helper function to validate received message
func validateMessage(t *testing.T, msg *Message) {
	assert.Equal(t, uint8(1), msg.Version)
	assert.Equal(t, uint8(0), msg.Status)
	assert.Equal(t, uint16(1000), msg.ServiceId)
	assert.Equal(t, uint16(200), msg.Cmd)
	assert.Equal(t, uint32(123456789), msg.Seq)
	assert.Equal(t, "Hello, World!", string(msg.Body))
}

// Test normal sending and receiving of messages
func TestSendReceive(t *testing.T) {
	server, client := net.Pipe() // 模拟TCP连接
	defer server.Close()
	defer client.Close()

	// 使用协议的 NewCodec 创建编码器和解码器
	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)
	clientCodec := protocol.NewCodec(client)

	// 构造一个测试消息
	testMsg := createTestMessage()

	// 客户端发送消息，服务器接收消息
	go func() {
		err := clientCodec.Send(testMsg)
		assert.NoError(t, err, "client send failed")
	}()

	receivedMsg, err := serverCodec.Receive()
	assert.NoError(t, err, "server receive failed")
	validateMessage(t, receivedMsg)
}

// Test message with maximum body size
func TestMaxBodySize(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)
	clientCodec := protocol.NewCodec(client)

	// 创建最大 Body 的消息
	maxBody := make([]byte, maxBodySize)
	for i := range maxBody {
		maxBody[i] = byte(i % 256) // 填充一些数据
	}
	testMsg := createTestMessage()
	testMsg.Body = maxBody

	go func() {
		err := clientCodec.Send(testMsg)
		assert.NoError(t, err, "client send failed")
	}()

	receivedMsg, err := serverCodec.Receive()
	assert.NoError(t, err, "server receive failed")
	assert.Equal(t, maxBody, receivedMsg.Body, "received max body does not match")
}

// Test empty body message
func TestEmptyBody(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)
	clientCodec := protocol.NewCodec(client)

	// 创建空 Body 的消息
	testMsg := createTestMessage()
	testMsg.Body = []byte{}

	go func() {
		err := clientCodec.Send(testMsg)
		assert.NoError(t, err, "client send failed")
	}()

	receivedMsg, err := serverCodec.Receive()
	assert.NoError(t, err, "server receive failed")
	assert.Empty(t, receivedMsg.Body, "received body is not empty")
}

// Test invalid packet size (too large)
func TestInvalidPacketSize(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)

	// 构造一个超出允许大小的假数据包
	invalidPacketSize := maxPackSize + 1
	packLenBuf := make([]byte, packSize)
	binary.BigEndian.PutUint32(packLenBuf, uint32(invalidPacketSize))

	// 构造一个无效的包：包头的长度符合要求，但总长度超限
	invalidPacket := append(packLenBuf, make([]byte, rawHeaderSize)...)

	go func() {
		client.Write(invalidPacket) // 写入不合法的数据包
	}()

	_, err := serverCodec.Receive()
	assert.Error(t, err, "expected error on invalid packet size")
	assert.Equal(t, ErrRawPackLen, err, "wrong error for invalid packet size")
}

// Test invalid header size
func TestInvalidHeaderSize(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)

	// 构造一个头部长度错误的数据包
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(rawHeaderSize+10)) // 设置错误头部长度
	binary.Write(&buf, binary.BigEndian, uint16(rawHeaderSize+1))  // 假设 header size 比实际大
	buf.Write(make([]byte, rawHeaderSize))                         // 正常的剩余头部

	go func() {
		client.Write(buf.Bytes()) // 直接写入无效数据
	}()

	serverCodec.SetReadDeadline(time.Now().Add(3 * time.Second)) // 防止错误的头导致 receive 阻塞
	_, err := serverCodec.Receive()
	//
	assert.Error(t, err, "expected error on invalid header size")
	assert.Equal(t, "read pipe: i/o timeout", err.Error(), "wrong error for invalid header size")
}

// Test deadlines for read/write operations
func TestReadWriteDeadlines(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	protocol := NewIMProtocol()
	serverCodec := protocol.NewCodec(server)
	clientCodec := protocol.NewCodec(client)

	// 设置读取超时为当前时间，立即超时
	err := serverCodec.SetReadDeadline(time.Now().Add(-time.Second))
	assert.NoError(t, err)

	_, err = serverCodec.Receive()
	assert.Error(t, err, "expected timeout error on receive")
	if err != nil && err.Error() != "read pipe: i/o timeout" {
		t.Errorf("unexpected error: %v", err)
	}

	// 设置写入超时
	err = clientCodec.SetWriteDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	testMsg := createTestMessage()
	err = clientCodec.Send(testMsg)
	if err == nil {
		t.Errorf("client send should timeout")
	}
	assert.Equal(t, "write pipe: i/o timeout", err.Error(), "client send should succeed within deadline")
}
