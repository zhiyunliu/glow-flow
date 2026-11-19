package goja

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var gojaLinePosRegx = regexp.MustCompile(`\(anonymous\): Line (\d+):(\d+)`)

func GetLineError(err error, lines int) error {
	if err == nil || lines == 0 {
		return err
	}
	msg := err.Error()
	m := gojaLinePosRegx.FindStringSubmatch(msg)
	if m == nil {
		return err
	}
	line, ierr := strconv.Atoi(m[1])
	if ierr != nil || line+lines < 1 {
		return err
	}
	return errors.New(gojaLinePosRegx.ReplaceAllString(msg, fmt.Sprintf("Line %d:$2", line+lines)))
}
