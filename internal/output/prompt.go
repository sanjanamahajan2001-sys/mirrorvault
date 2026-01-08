package output

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func AskProceedToBackup() bool {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\nDo you want to proceed with backup?")
	fmt.Println("1) Yes")
	fmt.Println("2) No (exit)")
	fmt.Print("Enter choice: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	return input == "1"
}
