package goblue

import (
	"bufio"
	_ "embed"
	"math/rand"
	"strings"
)

//go:embed stamps/kia
var kia string

//go:embed stamps/hyundai
var hyundai string

func unpack(source string) ([]string, error) {
	var res []string
	scanner := bufio.NewScanner(strings.NewReader(source))
	for scanner.Scan() {
		res = append(res, scanner.Text())
	}
	return res, scanner.Err()
}

func GetStampFromList(b Brand) (string, error) {
	switch b {
	case BrandHyundai:
		p, err := unpack(hyundai)
		if err != nil {
			return "", err
		}
		return p[rand.Intn(len(p))], nil
	case BrandKia:
		p, err := unpack(kia)
		if err != nil {
			return "", err
		}
		return p[rand.Intn(len(p))], nil
	}
	return "", ErrUnknownBrand
}
