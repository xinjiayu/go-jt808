package service

import (
	"errors"
	"fmt"
	"github.com/cuteLittleDevil/go-jt808/protocol/model"
	"github.com/cuteLittleDevil/go-jt808/shared/consts"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

type connection struct {
	conn                  *net.TCPConn
	handles               map[consts.JT808CommandType]Handler
	stopOnce              sync.Once
	stopChan              chan struct{}
	msgChan               chan *Message
	activeMsgChan         chan *ActiveMessage
	activeMsgCompleteChan chan *Message
	reissuePackChan       chan *Message
	// platformSerialNumber 平台流水号 到了math.MaxUint16后+1重新变成0
	platformSerialNumber uint16
	joinFunc             func(message *Message, activeChan chan<- *ActiveMessage) (string, error)
	leaveFunc            func(key string)
	key                  string
	filter               bool
	terminalEvent        TerminalEventer
}

func newConnection(conn *net.TCPConn, handles map[consts.JT808CommandType]Handler, terminalEvent TerminalEventer, filter bool,
	join func(message *Message, activeChan chan<- *ActiveMessage) (string, error), leave func(key string)) *connection {
	return &connection{
		conn:                  conn,
		handles:               handles,
		stopOnce:              sync.Once{},
		stopChan:              make(chan struct{}),
		msgChan:               make(chan *Message, 10),
		activeMsgChan:         make(chan *ActiveMessage, 3),
		activeMsgCompleteChan: make(chan *Message, 3),
		reissuePackChan:       make(chan *Message, 3),
		platformSerialNumber:  uint16(0),
		joinFunc:              join,
		leaveFunc:             leave,
		filter:                filter,
		terminalEvent:         terminalEvent,
	}
}

func (c *connection) Start() {
	go c.reader()
	go c.write()
}

func (c *connection) reader() {
	var (
		// 消息体长度最大为 10bit 也就是 1023 的字节
		curData = make([]byte, 1023)
		pack    = newPackageParse()
		join    = false
	)
	defer func() {
		c.stop()
		clear(curData)
		pack.clear()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			if n, err := c.conn.Read(curData); err != nil {
				if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
					slog.Debug("connection close",
						slog.Bool("join", join),
						slog.Any("platform num", c.platformSerialNumber),
						slog.Any("err", err))
					return
				}
				slog.Error("read data",
					slog.Bool("join", join),
					slog.Any("platform num", c.platformSerialNumber),
					slog.Any("err", err))
				return
			} else if n > 0 {
				effectiveData := curData[:n]
				msgs, err := pack.parse(effectiveData)
				if err != nil {
					slog.Error("parse data",
						slog.Bool("join", join),
						slog.Any("platform num", c.platformSerialNumber),
						slog.String("effective data", fmt.Sprintf("%x", effectiveData)),
						slog.Any("err", err))
					return
				}
				for _, msg := range msgs {
					if handler, ok := c.handles[msg.Command]; ok {
						msg.Handler = handler
						if msg.Command == consts.P8003ReissueSubcontractingRequest {
							c.reissuePackChan <- msg
							continue
						}
						c.onReadExecutionEvent(msg)
					} else {
						c.terminalEvent.OnNotSupportedEvent(msg)
						continue
					}
					if !join {
						key, err := c.joinFunc(msg, c.activeMsgChan)
						if err == nil {
							join = true
							c.key = key
						}
						c.terminalEvent.OnJoinEvent(msg, key, err)
						if errors.Is(err, _errKeyExist) {
							slog.Warn("key",
								slog.String("effective data", fmt.Sprintf("%x", effectiveData)),
								slog.Any("err", err))
							return
						}
					}
					c.msgChan <- msg
				}
			}
		}
	}
}

func (c *connection) write() {
	record := map[uint16]*ActiveMessage{}
	for {
		select {
		case <-c.stopChan:
			clear(record)
			return
		case activeMsg, ok := <-c.activeMsgChan: // 平台主动下发的
			if ok {
				c.onActiveEvent(activeMsg, record)
			}
		case msg, ok := <-c.activeMsgCompleteChan: // 平台主动下发的完成情况
			if ok {
				seq := msg.ExtensionFields.PlatformSeq
				if v, ok := record[seq]; ok {
					v.replyChan <- msg
					delete(record, seq)
				}
			}
		case subPackMsg, ok := <-c.reissuePackChan: // 分包补传的
			if ok {
				c.subPackReplyEvent(subPackMsg)
			}
		case msg, ok := <-c.msgChan: // 终端上传的
			if ok {
				if len(record) > 0 && msg.hasComplete() { // 说明现在有主动的请求 等待回复中
					if c.onActiveRespondEvent(record, msg) {
						continue
					}
				}
				c.defaultReplyEvent(msg)
			}
		}
	}
}

func (c *connection) stop() {
	c.stopOnce.Do(func() {
		c.leaveFunc(c.key)
		c.terminalEvent.OnLeaveEvent(c.key)
		clear(c.handles)
		close(c.msgChan)
		close(c.activeMsgChan)
		close(c.activeMsgCompleteChan)
		close(c.reissuePackChan)
		close(c.stopChan)
		_ = c.conn.Close()
	})
}

func (c *connection) defaultReplyEvent(msg *Message) {
	header := msg.JTMessage.Header
	header.ReplyID = uint16(msg.ReplyProtocol())
	seq := c.curSeq()
	header.PlatformSerialNumber = seq
	if has := msg.HasReply(); !has {
		return
	}
	body, err := msg.ReplyBody(msg.JTMessage)
	if err != nil {
		slog.Warn("reply body fail",
			slog.String("terminal data", fmt.Sprintf("%x", msg.ExtensionFields.TerminalData)),
			slog.Any("err", err))
		return
	}
	data := header.Encode(body)
	if _, err = c.conn.Write(data); err != nil {
		slog.Warn("write fail",
			slog.String("data", fmt.Sprintf("%x", data)),
			slog.Any("err", err))
		msg.ExtensionFields.Err = errors.Join(ErrWriteDataFail, err)
	}
	msg.ExtensionFields.PlatformSeq = seq
	msg.ExtensionFields.PlatformData = data
	c.onWriteExecutionEvent(msg)
}

func (c *connection) subPackReplyEvent(msg *Message) {
	header := msg.JTMessage.Header
	seq := c.curSeq()
	header.PlatformSerialNumber = seq
	header.ReplyID = uint16(consts.P8003ReissueSubcontractingRequest)
	data := header.Encode(msg.JTMessage.Body)
	if _, err := c.conn.Write(data); err != nil {
		slog.Warn("write fail",
			slog.String("data", fmt.Sprintf("%x", data)),
			slog.Any("err", err))
		msg.ExtensionFields.Err = errors.Join(ErrWriteDataFail, err)
	}
	msg.ExtensionFields.PlatformSeq = seq
	msg.ExtensionFields.PlatformData = data
	c.onWriteExecutionEvent(msg)
}

func (c *connection) onActiveEvent(activeMsg *ActiveMessage, record map[uint16]*ActiveMessage) {
	header := activeMsg.header
	seq := c.curSeq()
	header.PlatformSerialNumber = seq
	header.ReplyID = uint16(activeMsg.Command)
	data := header.Encode(activeMsg.Body)
	activeMsg.ExtensionFields = struct {
		PlatformSeq uint16 `json:"platformSeq,omitempty"`
		Data        []byte `json:"data,omitempty"`
	}{
		PlatformSeq: seq,
		Data:        data,
	}
	record[seq] = activeMsg
	_, err := c.conn.Write(data)
	if v, ok := c.handles[activeMsg.Command]; ok {
		msg := newActiveMessage(seq, activeMsg.Command, data, err)
		msg.Handler = v
		c.onWriteExecutionEvent(msg)
	}
	if err != nil {
		slog.Warn("write fail",
			slog.String("data", fmt.Sprintf("%x", data)),
			slog.Any("err", err))
		c.activeMsgCompleteChan <- newErrMessage(seq, errors.Join(ErrWriteDataFail, err))
		return
	}
	go func(msg *ActiveMessage, seq uint16) {
		duration := 5 * time.Second
		if msg.OverTimeDuration > 0 {
			duration = msg.OverTimeDuration
		}
		time.Sleep(duration)
		c.activeMsgCompleteChan <- newErrMessage(seq, errors.Join(ErrWriteDataOverTime,
			fmt.Errorf("overtime is [%.2f]second", duration.Seconds())))
	}(activeMsg, seq)
}

func (c *connection) onActiveRespondEvent(record map[uint16]*ActiveMessage, msg *Message) bool {
	type respond struct {
		JT808Handler
		HasRespondFunc func(seq uint16) bool
	}
	tmp := respond{
		JT808Handler:   nil,
		HasRespondFunc: nil,
	}
	switch msg.Command {
	case consts.T0001GeneralRespond:
		t0x0001 := &model.T0x0001{}
		tmp.JT808Handler = t0x0001
		tmp.HasRespondFunc = func(seq uint16) bool {
			return seq == t0x0001.SerialNumber
		}
	case consts.T0104QueryParameter:
		t0x0104 := &model.T0x0104{}
		tmp.JT808Handler = t0x0104
		tmp.HasRespondFunc = func(seq uint16) bool {
			return seq == t0x0104.RespondSerialNumber
		}
	case consts.T1003UploadAudioVideoAttr:
		t0x1003 := &model.T0x1003{}
		tmp.JT808Handler = t0x1003
		tmp.HasRespondFunc = func(_ uint16) bool {
			return true
		}
	case consts.T1205UploadAudioVideoResourceList:
		t0x1205 := &model.T0x1205{}
		tmp.JT808Handler = t0x1205
		tmp.HasRespondFunc = func(seq uint16) bool {
			return seq == t0x1205.SerialNumber
		}
	case consts.T1206FileUploadCompleteNotice:
		t0x1206 := &model.T0x1206{}
		tmp.JT808Handler = t0x1206
		tmp.HasRespondFunc = func(seq uint16) bool {
			return seq == t0x1206.RespondSerialNumber
		}
	}
	if tmp.HasRespondFunc != nil {
		if err := tmp.Parse(msg.JTMessage); err != nil {
			slog.Warn("parse fail",
				slog.String("terminal data", fmt.Sprintf("%x", msg.ExtensionFields.TerminalData)),
				slog.Any("err", err))
			return true
		}
		for k := range record {
			if tmp.HasRespondFunc(k) {
				msg.ExtensionFields.PlatformSeq = k
				msg.ExtensionFields.PlatformData = record[k].ExtensionFields.Data
				c.activeMsgCompleteChan <- msg
				return true
			}
		}
	}

	return false
}

func (c *connection) onReadExecutionEvent(msg *Message) {
	if c.filter && !msg.hasComplete() {
		return
	}
	if msg.Handler == nil {
		slog.Warn("Handler is nil",
			slog.String("read", msg.Header.String()))
		return
	}
	msg.Handler.OnReadExecutionEvent(msg)
	c.terminalEvent.OnReadExecutionEvent(msg)
}

func (c *connection) onWriteExecutionEvent(msg *Message) {
	if c.filter && !msg.hasComplete() {
		return
	}
	if msg.Handler == nil {
		slog.Warn("Handler is nil",
			slog.String("write", msg.Header.String()))
		return
	}
	msg.Handler.OnWriteExecutionEvent(*msg)
	c.terminalEvent.OnWriteExecutionEvent(*msg)
}

func (c *connection) curSeq() uint16 {
	defer func() {
		c.platformSerialNumber++
	}()
	return c.platformSerialNumber
}
