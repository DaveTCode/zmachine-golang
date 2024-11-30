package zmachine

type TextStyle int

const (
	Roman        TextStyle = 0b0000_0001
	Bold         TextStyle = 0b0000_0010
	Italic       TextStyle = 0b0000_0100
	ReverseVideo TextStyle = 0b0000_1000
	FixedPitch   TextStyle = 0b0001_0000
)

type Color int

const (
	Current     Color = 0
	Default     Color = 1
	Black       Color = 2
	Red         Color = 3
	Green       Color = 4
	Yellow      Color = 5
	Blue        Color = 6
	Magenta     Color = 7
	Cyan        Color = 8
	White       Color = 9
	LightGrey   Color = 10
	MediumGrey  Color = 11
	DarkGrey    Color = 12
	Reserved1   Color = 13
	Reserved2   Color = 14
	Transparent Color = 15
)

func (c Color) ToHex() string {
	switch c {
	case Black:
		return "#000000"
	case Red:
		return "#ff0000"
	case Green:
		return "#00ff00"
	case Yellow:
		return "#ffff00"
	case Blue:
		return "#0000ff"
	case Magenta:
		return "#ff00ff"
	case Cyan:
		return "#00ffff"
	case White:
		return "#ffffff"
	case LightGrey:
		return "#cccccc"
	case MediumGrey:
		return "#828282"
	case DarkGrey:
		return "#474747"
	default:
		panic("TODO - Handle other colours")
	}
}

// ScreenModel - This is very deliberately a _not_ V6 screen model
type ScreenModel struct {
	LowerWindowActive bool

	UpperWindowHeight     int
	UpperWindowForeground Color
	UpperWindowBackground Color
	UpperWindowCursorX    int
	UpperWindowCursorY    int
	UpperWindowTextStyle  TextStyle

	LowerWindowForeground Color
	LowerWindowBackground Color
	LowerWindowTextStyle  TextStyle
}

func newScreenModel(foregroundColor Color, backgroundColor Color) ScreenModel {
	return ScreenModel{
		LowerWindowActive:     true,
		UpperWindowHeight:     0,
		UpperWindowForeground: foregroundColor,
		UpperWindowBackground: backgroundColor,
		UpperWindowCursorX:    1,
		UpperWindowCursorY:    1,
		UpperWindowTextStyle:  Roman,
		LowerWindowForeground: backgroundColor,
		LowerWindowBackground: foregroundColor,
		LowerWindowTextStyle:  Roman,
	}
}
