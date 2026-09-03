package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kushalsubedi/deploya/detector"
)

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	dir := fs.String("dir", ".", "Project directory")
	job := fs.String("job", "", "Job to add")
	fs.Usage = func() {
		fmt.Println(`Usage: deploya add --job <job-name> [flags]

Adds a new job to your existing pipeline.

CI jobs (added to ci.yml):
  lint              Language-specific linting
  pr-validate       PR title and label validation
  security          Trivy vulnerability scan
  dependency-review Scan PR dependencies for vulnerabilities
  codeql            GitHub CodeQL static analysis
  notify-slack      Slack notification on failure

Standalone workflows (separate workflow file):
  stale             Auto-close stale issues and PRs (daily schedule)
  deploy-ec2        SSH deploy to AWS EC2
  deploy-ecs        Deploy to AWS ECS service
  deploy-k8s        Deploy to Kubernetes cluster

Flags:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  deploya add --job lint
  deploya add --job security
  deploya add --job pr-validate
  deploya add --job deploy-ec2
  deploya add --job deploy-k8s`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *job == "" {
		fs.Usage()
		return fmt.Errorf("--job is required")
	}

	switch *job {
	// ── CI jobs → append to ci.yml ──────────────────────────────
	case "lint", "security", "codeql", "pr-validate", "dependency-review", "notify-slack":
		return addToCIYml(*dir, *job)

	// ── Standalone workflows → separate workflow file ─────────────
	case "stale", "deploy-ec2", "deploy-ecs", "deploy-k8s":
		return addStandaloneWorkflow(*dir, *job)

	default:
		return fmt.Errorf("unknown job: %q\n\nRun 'deploya add --help' to see available jobs", *job)
	}
}

// ── CI yml jobs ─────────────────────────────────────────────────────────────

func addToCIYml(dir, job string) error {
	ciPath := filepath.Join(dir, ".github", "workflows", "ci.yml")
	if _, err := os.Stat(ciPath); err != nil {
		return fmt.Errorf("ci.yml not found — run 'deploya init' first")
	}

	content, err := os.ReadFile(ciPath)
	if err != nil {
		return fmt.Errorf("could not read ci.yml: %w", err)
	}

	// Duplicate job keys make the workflow invalid — refuse to add twice.
	if strings.Contains(string(content), "\n  "+job+":") {
		fmt.Printf("⚠️  Job '%s' already exists in ci.yml — skipping\n", job)
		return nil
	}

	snippet, secrets := ciJobSnippet(dir, job)
	updated := string(content) + snippet

	if err := os.WriteFile(ciPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("could not update ci.yml: %w", err)
	}

	fmt.Printf("✅ Added '%s' job to .github/workflows/ci.yml\n", job)
	printSecretHints(secrets)
	return nil
}

// ── Deploy workflow jobs ─────────────────────────────────────────────────────

func addStandaloneWorkflow(dir, job string) error {
	workflowDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		return fmt.Errorf("could not create workflows directory: %w", err)
	}

	filename, content, secrets := standaloneWorkflowContent(dir, job)
	outPath := filepath.Join(workflowDir, filename)

	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("⚠️  %s already exists — skipping\n", outPath)
		return nil
	}

	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("could not write workflow: %w", err)
	}

	fmt.Printf("✅ Created .github/workflows/%s\n", filename)
	printSecretHints(secrets)
	return nil
}

// ── CI job snippets ──────────────────────────────────────────────────────────

func ciJobSnippet(dir, job string) (snippet string, secrets []string) {
	switch job {
	case "lint":
		return `
  lint:
    name: Lint
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Run linter
        run: |
          # Uncomment for your language:
          # Go:     golangci-lint run ./...
          # Python: flake8 . --max-line-length=120
          # Node:   npx eslint .
          # Ruby:   bundle exec rubocop
          echo "Configure your linter above"
`, nil

	case "pr-validate":
		return `
  pr-validate:
    name: PR validation
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    permissions:
      pull-requests: read

    steps:
      - name: Validate PR title
        uses: amannn/action-semantic-pull-request@v5
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          types: |
            feat
            fix
            chore
            docs
            perf
            refactor
            test
          requireScope: false

      - name: Check for WIP
        if: contains(github.event.pull_request.title, 'WIP') || contains(github.event.pull_request.title, 'wip')
        run: |
          echo "❌ PR title contains WIP — please remove before merging"
          exit 1
`, nil

	case "security":
		return `
  security:
    name: Security scan
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@0.28.0
        with:
          scan-type: "fs"
          scan-ref: "."
          severity: "CRITICAL,HIGH"
          format: "table"
          exit-code: "1"
          ignore-unfixed: true
`, nil

	case "dependency-review":
		return `
  dependency-review:
    name: Dependency review
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Dependency review
        uses: actions/dependency-review-action@v4
        with:
          fail-on-severity: high
`, nil

	case "codeql":
		return `
  codeql:
    name: CodeQL analysis
    runs-on: ubuntu-latest
    permissions:
      security-events: write
      contents: read

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Initialize CodeQL
        uses: github/codeql-action/init@v3
        with:
          languages: ` + codeqlLanguage(dir) + `

      - name: Autobuild
        uses: github/codeql-action/autobuild@v3

      - name: Perform CodeQL analysis
        uses: github/codeql-action/analyze@v3
`, nil

	case "notify-slack":
		return `
  notify-slack:
    name: Slack notification
    runs-on: ubuntu-latest
    needs: [test]
    if: failure()

    steps:
      - name: Notify Slack on failure
        uses: slackapi/slack-github-action@v1.26.0
        with:
          payload: |
            {
              "blocks": [
                {
                  "type": "header",
                  "text": { "type": "plain_text", "text": "❌ Pipeline Failed — ${{ github.repository }}" }
                },
                {
                  "type": "section",
                  "fields": [
                    { "type": "mrkdwn", "text": "*Branch*\n${{ github.ref_name }}" },
                    { "type": "mrkdwn", "text": "*Triggered by*\n${{ github.actor }}" }
                  ]
                },
                {
                  "type": "actions",
                  "elements": [{
                    "type": "button",
                    "text": { "type": "plain_text", "text": "View Run" },
                    "url": "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}"
                  }]
                }
              ]
            }
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
          SLACK_WEBHOOK_TYPE: INCOMING_WEBHOOK
`, []string{"SLACK_WEBHOOK_URL"}
	}

	return "", nil
}

// ── Standalone workflow content ──────────────────────────────────────────────

func standaloneWorkflowContent(dir, job string) (filename, content string, secrets []string) {
	ciName := ciWorkflowName(dir)
	switch job {
	case "stale":
		return "stale.yml", `name: Close stale issues and PRs

on:
  schedule:
    - cron: "17 3 * * *"

jobs:
  stale:
    name: Mark stale issues and PRs
    runs-on: ubuntu-latest
    permissions:
      issues: write
      pull-requests: write

    steps:
      - name: Stale
        uses: actions/stale@v9
        with:
          stale-issue-message: "This issue has been inactive for 30 days and will be closed in 7 days."
          stale-pr-message: "This PR has been inactive for 30 days and will be closed in 7 days."
          stale-issue-label: "stale"
          stale-pr-label: "stale"
          days-before-stale: 30
          days-before-close: 7
`, nil

	case "deploy-ec2":
		return "deploy-ec2.yml", `name: Deploy to EC2

on:
  workflow_run:
    workflows: ["` + ciName + `"]
    branches: [main]
    types: [completed]

jobs:
  deploy:
    name: Deploy to EC2
    runs-on: ubuntu-latest
    if: ${{ github.event.workflow_run.conclusion == 'success' }}

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Deploy to EC2 via SSH
        uses: appleboy/ssh-action@v1.0.3
        with:
          host: ${{ secrets.EC2_HOST }}
          username: ${{ secrets.EC2_USER }}
          key: ${{ secrets.EC2_SSH_KEY }}
          script: |
            cd ${{ secrets.EC2_APP_DIR }}
            git pull origin main
            # Add your restart commands here:
            # sudo systemctl restart ${{ secrets.EC2_SERVICE }}
            # or: docker compose pull && docker compose up -d
            echo "✅ Deployed successfully"
`, []string{"EC2_HOST", "EC2_USER", "EC2_SSH_KEY", "EC2_APP_DIR"}

	case "deploy-ecs":
		return "deploy-ecs.yml", `name: Deploy to ECS

on:
  workflow_run:
    workflows: ["` + ciName + `"]
    branches: [main]
    types: [completed]

jobs:
  deploy:
    name: Deploy to ECS
    runs-on: ubuntu-latest
    if: ${{ github.event.workflow_run.conclusion == 'success' }}

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ secrets.AWS_REGION }}

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build and push image to ECR
        id: build-image
        env:
          ECR_REGISTRY: ${{ steps.login-ecr.outputs.registry }}
          IMAGE_TAG: ${{ github.sha }}
        run: |
          docker build -t $ECR_REGISTRY/${{ secrets.ECS_CONTAINER_NAME }}:$IMAGE_TAG .
          docker push $ECR_REGISTRY/${{ secrets.ECS_CONTAINER_NAME }}:$IMAGE_TAG
          echo "image=$ECR_REGISTRY/${{ secrets.ECS_CONTAINER_NAME }}:$IMAGE_TAG" >> $GITHUB_OUTPUT

      - name: Update ECS service
        run: |
          aws ecs update-service \
            --cluster ${{ secrets.ECS_CLUSTER }} \
            --service ${{ secrets.ECS_SERVICE }} \
            --force-new-deployment
`, []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "ECS_CLUSTER", "ECS_SERVICE", "ECS_CONTAINER_NAME"}

	case "deploy-k8s":
		return "deploy-k8s.yml", `name: Deploy to Kubernetes

on:
  workflow_run:
    workflows: ["` + ciName + `"]
    branches: [main]
    types: [completed]

jobs:
  deploy:
    name: Deploy to Kubernetes
    runs-on: ubuntu-latest
    if: ${{ github.event.workflow_run.conclusion == 'success' }}

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up kubectl
        uses: azure/setup-kubectl@v3

      - name: Configure kubeconfig
        run: |
          mkdir -p $HOME/.kube
          echo "${{ secrets.KUBECONFIG }}" | base64 -d > $HOME/.kube/config

      - name: Deploy to Kubernetes
        run: |
          kubectl set image deployment/${{ secrets.K8S_DEPLOYMENT }} \
            ${{ secrets.K8S_CONTAINER }}=${{ secrets.K8S_IMAGE }}:${{ github.sha }} \
            --namespace=${{ secrets.K8S_NAMESPACE }}
          kubectl rollout status deployment/${{ secrets.K8S_DEPLOYMENT }} \
            --namespace=${{ secrets.K8S_NAMESPACE }}
`, []string{"KUBECONFIG", "K8S_DEPLOYMENT", "K8S_CONTAINER", "K8S_IMAGE", "K8S_NAMESPACE"}
	}

	return "", "", nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// ciWorkflowName reads the workflow name from ci.yml so workflow_run
// triggers reference an existing workflow. Falls back to "CI".
func ciWorkflowName(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "ci.yml"))
	if err != nil {
		return "CI"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "name:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, `"'`)
			if name != "" {
				return name
			}
		}
	}
	return "CI"
}

// codeqlLanguage maps the detected project language to a CodeQL language id.
func codeqlLanguage(dir string) string {
	lang, _ := detector.DetectLanguage(dir)
	switch lang {
	case "go":
		return "go"
	case "python":
		return "python"
	case "node":
		return "javascript-typescript"
	case "java":
		return "java-kotlin"
	case "ruby":
		return "ruby"
	case "rust":
		return "rust"
	}
	return "go  # change to: python, javascript-typescript, java-kotlin, ruby"
}

func printSecretHints(secrets []string) {
	if len(secrets) == 0 {
		return
	}
	fmt.Println("\n🔑 Required secrets (GitHub → Settings → Secrets → Actions):")
	for _, s := range secrets {
		fmt.Printf("   • %s\n", s)
	}
}
