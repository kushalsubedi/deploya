# Hi, this is Deploya!

## What it does?
- It generates a `ci.yml` file that will get triggered on push or PR on main branch
- It will generate basic jobs like test, build, notification to your corporate messaging app (e.g. Slack, Discord, or your boss's email if required)
- Generates a build summary of CI runs

## Why?
Because we hate creating `ci.yml` and sometimes a basic one will also work. Yet it can be upgraded to handle some repetitive tasks.

## Who built this?
Of course me, and my free AI tools.

## How to run
```bash
go build .
# it will create deploya binary and you can run it with
deploya init
# or
go run main.go init
```

## What can it do?
It can detect some popular languages like Python, Go, JS/TS (Node), Java, Rust, Ruby based upon the directory and the files they have.

**Example:** it detects Python by looking into `pyproject.toml` or `requirements.txt` like files, and Go from `go.mod` file.

## How it works
1. Detects programming language
2. Asks user if they need webhook integration or not for messaging
3. Asks user if they are using any container registry — only available if the project has a visible `Dockerfile`
4. Creates `ci.yml`

## What can be added?
A lot of things.. will update about it later.