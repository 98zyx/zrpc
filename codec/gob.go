package codec

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn   io.ReadWriteCloser
	encBuf *bytes.Buffer
	decBuf *bytes.Buffer
	dec    *gob.Decoder
	enc    *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	encBuf, decBuf := &bytes.Buffer{}, &bytes.Buffer{}
	return &GobCodec{
		conn:   conn,   // 建立的连接
		encBuf: encBuf, // 缓存
		decBuf: decBuf,
		dec:    gob.NewDecoder(decBuf), // 直接从连接解码
		enc:    gob.NewEncoder(encBuf), // 直接通过连接编码发送conn
	}
}
func Int32ToBytes(i int32) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf, uint32(i))
	return buf
}

func BytesToInt32(buf []byte) int32 {
	return int32(binary.BigEndian.Uint32(buf))
}

func (c *GobCodec) ReadHeader(h *Header) error {
	lengthBytes := make([]byte, 4)
	_, err := c.conn.Read(lengthBytes)
	if err != nil {
		return err
	}
	length := BytesToInt32(lengthBytes)
	content := make([]byte, length)
	_, err = c.conn.Read(content)
	if err != nil {
		return err
	}
	c.decBuf.Write(content)
	return c.dec.Decode(h) // gob解码
}

func (c *GobCodec) ReadBody(body interface{}) error {
	lengthBytes := make([]byte, 4)
	_, err := c.conn.Read(lengthBytes)
	if err != nil {
		return err
	}
	length := BytesToInt32(lengthBytes)
	content := make([]byte, length)
	_, err = c.conn.Read(content)
	if err != nil {
		return err
	}
	c.decBuf.Write(content)
	return c.dec.Decode(body) // gob解码
}

func (c *GobCodec) Write(h *Header, body interface{}) error {
	// Encode()将会将h编码为gob格式并传输出去
	if err := c.enc.Encode(h); err != nil {
		c.encBuf.Reset()
		log.Println("rpc codec: gob error encoding header:", err)
		return err
	}
	length := int32(c.encBuf.Len())
	if err := binary.Write(c.conn, binary.BigEndian, &length); err != nil {
		c.encBuf.Reset()
		return err
	}
	if _, err := c.conn.Write(c.encBuf.Bytes()); err != nil {
		c.encBuf.Reset()
		return err
	}
	c.encBuf.Reset()
	// 发送body
	if err := c.enc.Encode(body); err != nil {
		c.encBuf.Reset()
		log.Println("rpc codec: gob error encoding body:", err)
		return err
	}
	length = int32(c.encBuf.Len())
	if err := binary.Write(c.conn, binary.BigEndian, &length); err != nil {
		c.encBuf.Reset()
		return err
	}
	if _, err := c.conn.Write(c.encBuf.Bytes()); err != nil {
		c.encBuf.Reset()
		return err
	}
	c.encBuf.Reset()
	return nil
}

func (c *GobCodec) Close() error { // 关闭连接
	return c.conn.Close()
}
