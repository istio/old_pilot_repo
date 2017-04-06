package model

import "fmt"

type ItemAlreadyExistsError struct {
	Key Key
	Msg string
}

func (e *ItemAlreadyExistsError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("Item with key %+v already exists", e.Key)
}

type ItemNotFoundError struct {
	Msg string
}

func (e *ItemNotFoundError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "item not found"
}
