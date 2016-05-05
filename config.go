package eeyore

import (
	"github.com/BurntSushi/toml"
	"io/ioutil"
)

func LoadConfig(filename string) Config {
	var config Config

	content, err := ioutil.ReadFile(filename)

	if err != nil {
		panic(err)
	}

	_, err = toml.Decode(string(content), &config)

	if err != nil {
		panic(err)
	}

	return config
}
