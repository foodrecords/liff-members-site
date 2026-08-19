package interpreter

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-playground/form"
	"github.com/go-playground/validator"
)

var (
	decoder  *form.Decoder
	validate *validator.Validate
)

func init() {
	decoder = form.NewDecoder()
	decoder.RegisterCustomTypeFunc(func(vs []string) (interface{}, error) {
		return time.Parse("2006-01-02", vs[0])
	}, time.Time{})

	validate = validator.New()
}

func Interpret(r *http.Request, v interface{}) error {
	if err := Decode(r, v); err != nil {
		return err
	}

	if err := Validate(v); err != nil {
		return err
	}

	return nil
}

func Decode(r *http.Request, v interface{}) error {
	if r.Method == http.MethodGet || r.Method == http.MethodDelete {
		return decoder.Decode(v, r.URL.Query())
	} else {
		return json.NewDecoder(r.Body).Decode(v)
	}
}

func Validate(v interface{}) error {
	return validate.Struct(v)
}
