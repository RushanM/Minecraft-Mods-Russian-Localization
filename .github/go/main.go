package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// Определение флагов
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	updateCmd := flag.NewFlagSet("update", flag.ExitOnError)

	// Проверка аргументов командной строки
	if len(os.Args) < 2 {
		fmt.Println("Ожидается подкоманда 'generate' или 'update'")
		os.Exit(1)
	}

	// Выбор команды
	switch os.Args[1] {
	case "generate":
		generateCmd.Parse(os.Args[2:])
		GenerateModList()
	case "update":
		updateCmd.Parse(os.Args[2:])
		UpdateReadme()
	default:
		fmt.Printf("Неизвестная подкоманда: %s\n", os.Args[1])
		os.Exit(1)
	}
}
