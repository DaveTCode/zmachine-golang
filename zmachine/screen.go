package zmachine

import "fmt"

type TextStyle int

const (
	Roman        TextStyle = 0b0000_0001
	Bold         TextStyle = 0b0000_0010
	Italic       TextStyle = 0b0000_0100
	ReverseVideo TextStyle = 0b0000_1000
	FixedPitch   TextStyle = 0b0001_0000
)

type Color struct {
	r int
	g int
	b int
}

func (c Color) ToHex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

// Font represents the available Z-machine fonts
type Font uint16

const (
	FontNormal     Font = 1
	FontPicture    Font = 2
	FontCharGraphs Font = 3
	FontFixedPitch Font = 4
)

// ScreenModel - This is very deliberately a _not_ V6 screen model
type ScreenModel struct {
	LowerWindowActive bool
	CurrentFont       Font // TODO - Not actually changing the rendering code based on this at the moment

	UpperWindowHeight            int
	UpperWindowForeground        Color
	UpperWindowBackground        Color
	DefaultUpperWindowForeground Color
	DefaultUpperWindowBackground Color
	UpperWindowCursorX           int
	UpperWindowCursorY           int
	UpperWindowTextStyle         TextStyle

	DefaultLowerWindowForeground Color
	DefaultLowerWindowBackground Color
	LowerWindowForeground        Color
	LowerWindowBackground        Color
	LowerWindowTextStyle         TextStyle
}

func (m *ScreenModel) NewZMachineColor(i uint16, isForeground bool) Color {
	switch i {
	case 0: // CURRENT
		if isForeground {
			return m.LowerWindowForeground
		} else {
			return m.LowerWindowBackground
		}
	case 1: // DEFAULT - TODO - Maybe make these defaults set in the screen model on creation?
		if isForeground {
			if m.LowerWindowActive {
				return m.DefaultLowerWindowForeground
			} else {
				return m.DefaultUpperWindowForeground
			}
		} else {
			if m.LowerWindowActive {
				return m.DefaultLowerWindowBackground
			} else {
				return m.DefaultUpperWindowBackground
			}
		}
	case 2: // BLACK
		return Color{0, 0, 0}
	case 3: // RED
		return Color{255, 0, 0}
	case 4: // GREEN
		return Color{0, 255, 0}
	case 5: // YELLOW
		return Color{255, 255, 0}
	case 6: // BLUE
		return Color{0, 0, 255}
	case 7: // MAGENTA
		return Color{255, 0, 255}
	case 8: // CYAN
		return Color{0, 255, 255}
	case 9: // WHITE
		return Color{255, 255, 255}
	case 10: // LIGHT GREY
		return Color{192, 192, 192}
	case 11: // MEDIUM GREY
		return Color{128, 128, 128}
	case 12: // DARK GREY
		return Color{64, 64, 64}
	default:
		//panic("TODO - Handle other colours")
		return Color{0, 0, 0}
	}
}

func newScreenModel(foregroundColor Color, backgroundColor Color) ScreenModel {
	return ScreenModel{
		LowerWindowActive:            true,
		CurrentFont:                  FontNormal,
		UpperWindowHeight:            0,
		DefaultUpperWindowForeground: foregroundColor,
		DefaultUpperWindowBackground: backgroundColor,
		UpperWindowForeground:        foregroundColor,
		UpperWindowBackground:        backgroundColor,
		UpperWindowCursorX:           1,
		UpperWindowCursorY:           1,
		UpperWindowTextStyle:         Roman,
		DefaultLowerWindowForeground: backgroundColor,
		DefaultLowerWindowBackground: foregroundColor,
		LowerWindowForeground:        backgroundColor,
		LowerWindowBackground:        foregroundColor,
		LowerWindowTextStyle:         Roman,
	}
}
