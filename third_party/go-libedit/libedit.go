package libedit

import (
	"errors"
	"io"
)

var ErrInterrupted = errors.New("interrupted")

type EditLine interface {
	Close()
	RebindControlKeys()
	UseHistory(int, bool) error
	LoadHistory(string) error
	SetLeftPrompt(string)
	SetAutoSaveHistory(string, bool)
	GetLine() (string, error)
	AddHistory(string) error
}

type dummyEditLine struct{}

func Init(_ string, _ bool) (EditLine, error) {
	return dummyEditLine{}, nil
}

func (dummyEditLine) Close()                          {}
func (dummyEditLine) RebindControlKeys()              {}
func (dummyEditLine) UseHistory(int, bool) error      { return nil }
func (dummyEditLine) LoadHistory(string) error        { return nil }
func (dummyEditLine) SetLeftPrompt(string)            {}
func (dummyEditLine) SetAutoSaveHistory(string, bool) {}
func (dummyEditLine) GetLine() (string, error)        { return "", io.EOF }
func (dummyEditLine) AddHistory(string) error         { return nil }
