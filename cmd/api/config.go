package main

import (
	"log"
	"reflect"

	"github.com/caarlos0/env/v6"
)

type Config struct {
	API struct {
		Port        int `env:"PORT" envDefault:"7077"`
		MetricsPort int `env:"METRICS_PORT" envDefault:"9010"`
	}

	App struct {
		LogLevel               string `env:"LOG_LEVEL" envDefault:"INFO"`
		AirdropDataBocFilename string `env:"AIRDROP_FILE,required"`
		JettonMaster           string `env:"JETTON_MASTER,required"`
	}
}

func Load() Config {
	var c Config
	if err := env.ParseWithFuncs(&c, map[reflect.Type]env.ParserFunc{}); err != nil {
		log.Panicf("[‼️  Config parsing failed] %+v\n", err)
	}
	return c
}
