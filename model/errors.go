package model

type ItemAlreadyExistsError struct {
	Msg string
}

func (e *ItemAlreadyExistsError) Error() string {
	return e.Msg
}

type ItemNotFoundError struct {
	Msg string
}

func (e *ItemNotFoundError) Error() string {
	return e.Msg
}
