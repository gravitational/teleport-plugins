package lib

import "fmt"

func ExampleMarkdownEscape() {
	fmt.Printf("%q\n", MarkdownEscape("     ", 1000))
	fmt.Printf("%q\n", MarkdownEscape("abc", 1000))
	fmt.Printf("%q\n", MarkdownEscape("`foo` `bar`", 1000))
	fmt.Printf("%q\n", MarkdownEscape("  123456789012345  ", 10))

	// Output: "(empty)"
	// "```\nabc```"
	// "```\n`\ufefffoo`\ufeff `\ufeffbar`\ufeff```"
	// "```\n1234567890``` (truncated)"
}
