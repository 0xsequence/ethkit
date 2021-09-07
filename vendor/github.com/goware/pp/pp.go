package pp

import (
	"bytes"
	"fmt"
	"os"
)

var DisableColors = false

// cW(buf, false, bRed, "\n")
// cW(buf, useColor, bCyan, " panic: ")
// cW(buf, useColor, bBlue, "%v", rvr)
// cW(buf, false, bWhite, "\n \n")

type PP struct {
	buf      *bytes.Buffer
	useColor bool
}

func NewPP() *PP {
	return &PP{
		buf:      &bytes.Buffer{},
		useColor: !DisableColors,
	}
}

func (p *PP) Println() {
	fmt.Fprintf(p.buf, "\n")
	os.Stdout.Write(p.buf.Bytes())
}

func (p *PP) Black(msg string, args ...interface{}) *PP {
	return p.colorPrint(bBlack, msg, args...)
}

func (p *PP) Red(msg string, args ...interface{}) *PP {
	return p.colorPrint(bRed, msg, args...)
}

func (p *PP) Green(msg string, args ...interface{}) *PP {
	return p.colorPrint(bGreen, msg, args...)
}

func (p *PP) Yellow(msg string, args ...interface{}) *PP {
	return p.colorPrint(bYellow, msg, args...)
}

func (p *PP) Blue(msg string, args ...interface{}) *PP {
	return p.colorPrint(bBlue, msg, args...)
}

func (p *PP) Magenta(msg string, args ...interface{}) *PP {
	return p.colorPrint(bMagenta, msg, args...)
}

func (p *PP) Cyan(msg string, args ...interface{}) *PP {
	return p.colorPrint(bCyan, msg, args...)
}

func (p *PP) White(msg string, args ...interface{}) *PP {
	return p.colorPrint(bWhite, msg, args...)
}

func (p *PP) colorPrint(color []byte, msg string, args ...interface{}) *PP {
	if p.buf.Len() > 0 {
		fmt.Fprintf(p.buf, " ")
	}
	cW(p.buf, p.useColor, color, msg, args...)
	return p
}

func Black(msg string, args ...interface{}) *PP {
	return NewPP().Black(msg, args...)
}

func Red(msg string, args ...interface{}) *PP {
	return NewPP().Red(msg, args...)
}

func Green(msg string, args ...interface{}) *PP {
	return NewPP().Green(msg, args...)
}

func Yellow(msg string, args ...interface{}) *PP {
	return NewPP().Yellow(msg, args...)
}

func Blue(msg string, args ...interface{}) *PP {
	return NewPP().Blue(msg, args...)
}

func Magenta(msg string, args ...interface{}) *PP {
	return NewPP().Magenta(msg, args...)
}

func Cyan(msg string, args ...interface{}) *PP {
	return NewPP().Cyan(msg, args...)
}

func White(msg string, args ...interface{}) *PP {
	return NewPP().White(msg, args...)
}
