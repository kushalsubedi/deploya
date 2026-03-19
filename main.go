// package main

// import (
// 	"deploya/detector"
// 	"deploya/generator"
// 	"fmt"
// 	"os"
// )

// func main() {
// 	dir := "."
// 	if len(os.Args) > 1 {
// 		dir = os.Args[1]
// 	}

// 	ctx := detector.Detect(dir)

// 	fmt.Println("📋 Detection results:")
// 	fmt.Printf("   Language     : %s %s\n", ctx.Language, ctx.Runtime)
// 	fmt.Printf("   Docker       : %v (compose: %v)\n", ctx.HasDocker, ctx.HasCompose)
// 	fmt.Printf("   Test command : %s\n", ctx.TestCommand)
// 	fmt.Printf("   Cloud        : %s\n", ctx.Cloud)
// 	fmt.Printf("   Main branch  : %s\n", ctx.MainBranch)
// 	fmt.Printf("   Repo name    : %s\n", ctx.RepoName)

// 	fmt.Println("\n📄 Generated pipeline:")
// 	fmt.Println("─────────────────────────────────────────")

// 	content, err := generator.Generate(ctx, dir)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "error: %v\n", err)
// 		os.Exit(1)
// 	}
// 	fmt.Println(content)
// }

package main

import (
	"fmt"
	"os"

	"deploya/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
