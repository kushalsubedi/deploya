package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Choice struct {
	Key   string
	Label string
}

// Select shows a numbered menu and returns the chosen key.
// If defaultKey is provided and user presses enter, that key is used.
func Select(question string, choices []Choice, defaultKey string) string {
	fmt.Println()
	fmt.Printf("  %s\n", question)
	for i, c := range choices {
		marker := " "
		if c.Key == defaultKey {
			marker = "*"
		}
		fmt.Printf("  %s %d) %s\n", marker, i+1, c.Label)
	}

	defaultLabel := ""
	for _, c := range choices {
		if c.Key == defaultKey {
			defaultLabel = c.Label
			break
		}
	}

	for {
		if defaultKey != "" {
			fmt.Printf("\n  Enter choice [default: %s]: ", defaultLabel)
		} else {
			fmt.Print("\n  Enter choice: ")
		}

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// User pressed enter — use default
		if input == "" && defaultKey != "" {
			fmt.Printf("  → %s\n", defaultLabel)
			return defaultKey
		}

		// Try parsing as number
		n, err := strconv.Atoi(input)
		if err == nil && n >= 1 && n <= len(choices) {
			chosen := choices[n-1]
			fmt.Printf("  → %s\n", chosen.Label)
			return chosen.Key
		}

		// Try matching by key directly
		for _, c := range choices {
			if strings.EqualFold(input, c.Key) {
				fmt.Printf("  → %s\n", c.Label)
				return c.Key
			}
		}

		fmt.Printf("  ⚠️  Invalid choice. Enter a number between 1 and %d.\n", len(choices))
	}
}

// Confirm asks a yes/no question. defaultYes controls the default.
func Confirm(question string, defaultYes bool) bool {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Printf("\n  %s [%s]: ", question, hint)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}
