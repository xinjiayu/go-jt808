package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cuteLittleDevil/go-jt808/protocol"
	"github.com/cuteLittleDevil/go-jt808/protocol/jt808"
	"github.com/cuteLittleDevil/go-jt808/shared/consts"
	"reflect"
	"testing"
)

func TestReply(t *testing.T) {
	type Handler interface {
		HasReply() bool
		ReplyBody(*jt808.JTMessage) ([]byte, error)
		ReplyProtocol() consts.JT808CommandType
	}
	type args struct {
		Handler
		msg2011 string
		msg2013 string
		msg2019 string
	}
	type want struct {
		result2011 string
		result2013 string
		result2019 string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 测试的数据使用terminal.go中的CreateTerminalPackage生成
			// 终端和平台的流水号都使用0
			name: "T0x0002 终端-心跳",
			args: args{
				Handler: &T0x0002{},
				msg2013: "7e000200000123456789017fff0a7e",
				msg2019: "7e000240000100000000017299841738ffff027e",
			},
			want: want{
				result2013: "7e8001000501234567890100007fff0002008e7e",
				result2019: "7e8001400501000000000172998417380000ffff000200867e",
			},
		},
		{
			name: "T0x0001 终端-通用应答",
			args: args{
				Handler: &T0x0001{},
				msg2013: "7e000100050123456789017fff007b01c803bd7e",
			},
			want: want{
				result2013: "7e8001000501234567890100007fff0001008d7e",
			},
		},
		{
			name: "T0x0102 终端-鉴权",
			args: args{
				Handler: &T0x0102{},
				msg2013: "7e0102000b01234567890100003137323939383431373338b57e",
				msg2019: "7e0102402f010000000001729984173800000b3137323939383431373338313233343536373839303132333435332e372e31350000000000000000000000000000227e",
			},
			want: want{
				result2013: "7e80010005012345678901000000000102010e7e",
				result2019: "7e80014005010000000001729984173800000000010200877e",
			},
		},
		{
			name: "T0x0100 终端-注册",
			args: args{
				Handler: &T0x0100{},
				msg2011: "7e010000200123456789010000001f007363640000007777772e3830382e3736353433323101b2e24131323334a17e",
				msg2013: "7e0100002c0123456789010000001f007363640000007777772e3830382e636f6d0000000000000000003736353433323101b2e24131323334cc7e",
				msg2019: "7e0100405301000000000172998417380000001f007363640000000000000000007777772e3830382e636f6d0000000000000000000000000000000000000037363534333231000000000000000000000000000000000000000000000001b2e241313233343b7e",
			},
			want: want{
				result2011: "7e8100000e01234567890100000000003132333435363738393031377e",
				result2013: "7e8100000e01234567890100000000003132333435363738393031377e",
				result2019: "7e8100400e010000000001729984173800000000003137323939383431373338ba7e",
			},
		},
		{
			name: "T0x0200 终端-位置信息",
			args: args{
				Handler: &T0x0200{},
				msg2013: "7e0200007c0123456789017fff000004000000080006eeb6ad02633df701380003006320070719235901040000000b02020016030200210402002c051e3737370000000000000000000000000000000000000000000000000000001105420000004212064d0000004d4d1307000000580058582504000000632a02000a2b040000001430011e3101286b7e",
				msg2019: "7e0200407c0100000000017299841738ffff000004000000080006eeb6ad02633df701380003006320070719235901040000000b02020016030200210402002c051e3737370000000000000000000000000000000000000000000000000000001105420000004212064d0000004d4d1307000000580058582504000000632a02000a2b040000001430011e310128637e",
			},
			want: want{
				result2013: "7e8001000501234567890100007fff0200008e7e",
				result2019: "7e8001400501000000000172998417380000ffff020000867e",
			},
		},
		{
			name: "T0x0704 终端-位置信息批量上传",
			args: args{
				Handler: &T0x0704{},
				msg2013: "7e0704005d0123456789017fff000301001c000004000000080006eeb6ad02633df7013800030063200707192359001c000004000000080006eeb6ad02633df7013800030063200707192359001c000004000000080006eeb6ad02633df7013800030063200707192359067e",
				msg2019: "7e0704405d0100000000017299841738ffff000301001c000004000000080006eeb6ad02633df7013800030063200707192359001c000004000000080006eeb6ad02633df7013800030063200707192359001c000004000000080006eeb6ad02633df70138000300632007071923590e7e",
			},
			want: want{
				result2013: "7e8001000501234567890100007fff0704008f7e",
				result2019: "7e8001400501000000000172998417380000ffff070400877e",
			},
		},
		{
			name: "T0x0104 终端-查询参数",
			args: args{
				Handler: &T0x0104{},
				msg2019: "7E010443A20100000000014419999999000500045B00000001040000000A00000002040000003C00000003040000000200000004040000003C00000005040000000200000006040000003C000000070400000002000000100B31333031323334353637300000001105313233343500000012053132333435000000130E3132372E302E302E313A37303030000000140531323334350000001505313233343500000016053132333435000000170531323334350000001A093132372E302E302E310000001B04000004570000001C04000004580000001D093132372E302E302E310000002004000000000000002104000000000000002204000000000000002301300000002401300000002501300000002601300000002704000000000000002804000000000000002904000000000000002C04000003E80000002D04000003E80000002E04000003E80000002F04000003E800000030040000000A0000003102003C000000320416320A1E000000400B3133303132333435363731000000410B3133303132333435363732000000420B3133303132333435363733000000430B3133303132333435363734000000440B3133303132333435363735000000450400000001000000460400000000000000470400000000000000480B3133303132333435363738000000490B313330313233343536373900000050040000000000000051040000000000000052040000000000000053040000000000000054040000000000000055040000003C000000560400000014000000570400003840000000580400000708000000590400001C200000005A040000012C0000005B0200500000005C0200050000005D02000A0000005E02001E00000064040000000100000065040000000100000070040000000100000071040000006F000000720400000070000000730400000071000000740400000072000000751500030190320000002800030190320000002800050100000076130400000101000002020000030300000404000000000077160101000301F43200000028000301F43200000028000500000079032808010000007A04000000230000007B0232320000007C1405000000000000000000000000000000000000000000008004000000240000008102000B000000820200660000008308BEA9415830303031000000840101000000900102000000910101000000920101000000930400000001000000940100000000950400000001000001000400000064000001010213880000010204000000640000010302138800000110080000000000000101F07E",
			},
		},
		{
			name: "P0x9003 平台-查询终端音视频属性",
			args: args{
				Handler: &P0x9003{},
				msg2013: "7e9003400001000000000144199999990003147e",
			},
		},
		{
			name: "T0x1003 终端-上传音视频属性",
			args: args{
				Handler: &T0x1003{},
				msg2013: "7e1003000a12345678901200017f040200944901200808177e",
			},
			want: want{
				result2013: "7e0000000012345678901200008a7e",
			},
		},
		{
			name: "P0x9101 平台-实时音视频传输请求",
			args: args{
				Handler: &P0x9101{},
				msg2013: "7e9101001712345678901200010f3132332e3132332e3132332e313233030440c60c0100a17e",
			},
		},
		{
			name: "P0x8104 平台-查询终端参数",
			args: args{
				Handler: &P0x8104{},
				msg2013: "7e8104400001000000000144199999990003027e",
			},
		},
		{
			name: "P0x9102 平台-音视频实时传输控制",
			args: args{
				Handler: &P0x9102{},
				msg2013: "7e9101001712345678901200010f3132332e3132332e3132332e313233030440c60c0100a17e",
			},
		},
		{
			name: "P0x9201 平台-下发远程录像回放请求",
			args: args{
				Handler: &P0x9201{},
				msg2013: "7e9201002412345678901200010d31322e31322e3132332e313233a7b93c6c320200000000200707192359200707192359617e",
			},
		},
		{
			name: "P0x9205 平台-查询资源列表",
			args: args{
				Handler: &P0x9205{},
				msg2013: "7e920500181234567890120001e720070719235920070719235900000000000000009b6e00167e",
			},
		},
		{
			name: "T0x1205 终端-上传音视频资源列表",
			args: args{
				Handler: &T0x1205{},
				msg2013: "7e1205002212345678901200000000000000010124110200000024110200010200000000000004000101010000000bb27e",
			},
		},
		{
			name: "P0x9207 平台-文件上传控制",
			args: args{
				Handler: &P0x9207{},
				msg2013: "7e92070003123456789012000169fd028b7e",
			},
		},
		{
			name: "P0x9206 平台-文件上传指令",
			args: args{
				Handler: &P0x9206{},
				msg2013: "7e9206004512345678901200010b3139322e3136382e312e312b2d08757365726e616d650870617373776f72640b2f616c61726d5f66696c6501200726000000200726232359000000000000000000010101227e",
			},
		},
		{
			name: "T0x1206 终端-文件上传完成通知",
			args: args{
				Handler: &T0x1206{},
				msg2013: "7e120640030112345678901234567890ffff1b8a01c67e",
			},
		},
		{
			name: "T0x0001 终端-通用应答",
			args: args{
				Handler: &T0x0001{},
				msg2013: "7e000100050123456789017fff007b01c803bd7e",
			},
		},
		{
			name: "P0x8003 平台-补发分包请求",
			args: args{
				Handler: &P0x8003{},
				msg2013: "7e800300150123456789017fff1099090001000200030004000500060007000800091f7e",
			},
		},
		{
			name: "P0x9105 平台-音视频实时传输状态通知",
			args: args{
				Handler: &P0x9105{},
				msg2013: "7e91050002123456789012000102031c7e",
			},
			want: want{
				result2013: "7e0001000012345678901200008b7e",
			},
		},
	}
	checkReplyInfo := func(t *testing.T, msg string, handler Handler, expectedResult string) {
		if msg == "" {
			return
		}
		data, _ := hex.DecodeString(msg)
		jtMsg := jt808.NewJTMessage()
		if err := jtMsg.Decode(data); err != nil {
			t.Errorf("Parse() error = %v", err)
			return
		}
		jtMsg.Header.ReplyID = uint16(handler.ReplyProtocol())
		if ok := handler.HasReply(); !ok {
			return
		}
		body, _ := handler.ReplyBody(jtMsg)
		got := jtMsg.Header.Encode(body)
		if !reflect.DeepEqual(fmt.Sprintf("%x", got), expectedResult) {
			t.Errorf("ReplyInfo() got = [%x]\n want = [%s]", got, expectedResult)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkReplyInfo(t, tt.args.msg2011, tt.args.Handler, tt.want.result2011)
			checkReplyInfo(t, tt.args.msg2013, tt.args.Handler, tt.want.result2013)
			checkReplyInfo(t, tt.args.msg2019, tt.args.Handler, tt.want.result2019)
		})
	}
}

// 为了覆盖率100%增加的测试 ------------------------------------
func TestT0x0102Reply(t *testing.T) {
	msg := "7e0102402f010000000001729984173800000b3137323939383431373338313233343536373839303132333435332e372e31350000000000000000000000000000227e"
	data, _ := hex.DecodeString(msg)
	jtMsg := jt808.NewJTMessage()
	_ = jtMsg.Decode(data)
	handler := &T0x0102{}
	// 强制错误情况
	jtMsg.Body = nil
	if _, err := handler.ReplyBody(jtMsg); !errors.Is(err, protocol.ErrBodyLengthInconsistency) {
		t.Errorf("T0x0102 ReplyBody() err[%v]", err)
		return
	}
}
