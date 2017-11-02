package main

import (
	"log"
)

var verbosity = 4

func Vf(level int, format string, v ...interface{}) {
	if level <= verbosity {
		log.Printf(format, v...)
	}
}
func V(level int, v ...interface{}) {
	if level <= verbosity {
		log.Print(v...)
	}
}
func Vln(level int, v ...interface{}) {
	if level <= verbosity {
		log.Println(v...)
	}
}
