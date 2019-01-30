package main

import (
	"fmt"
	"regexp"

	ct "github.com/daviddengcn/go-colortext"
)

var (
	ConsoleLog  = Messager{}
	markupRegex = regexp.MustCompile(`(?ms)<([a-zA-Z_=,;]+)>(?s:(.*?))<\/>`)

	nocolor = app.Flag("nocolor", "Disable coloring").Bool()
)

type Messager struct{}

// Basically a style sheet for the console.
func renderTheme(tag string) {
	if *nocolor {
		return
	}

	switch tag {
	case "important":
		ct.ChangeColor(ct.Black, true, ct.Red, true)

	case "doc":
		ct.Foreground(ct.Green, true)

	case "required", "repeated":
		ct.Foreground(ct.Red, true)

	case "type":
		ct.Foreground(ct.Cyan, true)

	case "name":
		ct.Foreground(ct.Yellow, true)

	case "keyword":
		ct.Foreground(ct.Blue, true)

	case "VQL":
		ct.Foreground(ct.Red, false)
	}
}

func (self Messager) Markup(mu string) {
	offset := 0
	start := 0
	end := 0

	matched := markupRegex.FindAllStringSubmatchIndex(mu, -1)
	for _, item := range matched {
		start, end = item[0], item[1]
		if start > offset {
			fmt.Print(mu[offset:start])
		}
		offset = end

		tag := mu[item[2]:item[3]]
		content := mu[item[4]:item[5]]

		renderTheme(tag)
		fmt.Print(content)
		ct.ResetColor()
	}

	// Print the end bit if we need to.
	if len(mu) > end {
		fmt.Print(mu[end:len(mu)])
	}
}

func (self Messager) Info(format string, args ...interface{}) {
	if *nocolor {
		fmt.Printf(format, args...)
		return
	}

	ct.Foreground(ct.Green, false)
	fmt.Printf(format, args...)
	ct.ResetColor()
}

func (self Messager) Warn(format string, args ...interface{}) {
	if *nocolor {
		fmt.Printf(format, args...)
		return
	}

	ct.Foreground(ct.Yellow, false)
	fmt.Printf(format, args...)
	ct.ResetColor()
}

func (self Messager) Error(format string, args ...interface{}) {
	if *nocolor {
		fmt.Printf(format, args...)
		return
	}

	ct.Foreground(ct.Red, false)
	fmt.Printf(format, args...)
	ct.ResetColor()
}
