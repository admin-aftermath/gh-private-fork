package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/go-gh"
	"github.com/spf13/cobra"
)

const defaultRemoteName = "origin"

type PrivateForkOptions struct {
	Repository        string
	GitArgs          []string
	Clone            bool
	Remote           bool
	PromptClone      bool
	PromptRemote     bool
	RemoteName       string
	Organization     string
	ForkName         string
	DefaultBranchOnly bool
}

func execGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	opts := &PrivateForkOptions{
		RemoteName: defaultRemoteName,
	}

	rootCmd := &cobra.Command{
		Use:   "private-fork [<repository>] [-- <gitflags>...]",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() == 0 && len(args[1:]) > 0 {
				return fmt.Errorf("repository argument required when passing git clone flags")
			}
			return nil
		},
		Short: "Create a private fork of a repository",
		Long: heredoc.Docf(`
			Create a private fork of a repository.

			With no argument, creates a private fork of the current repository. Otherwise, forks
			the specified repository.

			By default, the new fork is set to be your %[1]sorigin%[1]s remote and any existing
			origin remote is renamed to %[1]supstream%[1]s. To alter this behavior, you can set
			a name for the new fork's remote with %[1]s--remote-name%[1]s.

			The %[1]supstream%[1]s remote will be set as the default remote repository.

			Additional %[1]sgit clone%[1]s flags can be passed after %[1]s--%[1]s.
		`, "`"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Repository = args[0]
				opts.GitArgs = args[1:]
			}

			return privateForkRun(opts)
		},
	}

	rootCmd.Flags().BoolVar(&opts.Clone, "clone", false, "Clone the fork")
	rootCmd.Flags().BoolVar(&opts.Remote, "remote", false, "Add a git remote for the fork")
	rootCmd.Flags().StringVar(&opts.RemoteName, "remote-name", defaultRemoteName, "Specify the name for the new remote")
	rootCmd.Flags().StringVar(&opts.Organization, "org", "", "Create the fork in an organization")
	rootCmd.Flags().StringVar(&opts.ForkName, "fork-name", "", "Rename the forked repository")
	rootCmd.Flags().BoolVar(&opts.DefaultBranchOnly, "default-branch-only", false, "Only include the default branch in the fork")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func privateForkRun(opts *PrivateForkOptions) error {
	// Get source repository
	sourceRepo := ""
	if opts.Repository == "" {
		// Get current repository if none specified
		stdout, stderr, err := gh.Exec("repo", "view", "--json", "url")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", stderr.String())
			return fmt.Errorf("unable to determine current repository: %w", err)
		}
		sourceRepo = strings.TrimSpace(stdout.String())
	} else {
		sourceRepo = opts.Repository
	}

	// Parse repository URL/name
	repoToFork, err := parseRepository(sourceRepo)
	if err != nil {
		return err
	}

	// Create bare clone
	fmt.Printf("Creating bare clone of %s...\n", sourceRepo)
	if err := execGit("clone", "--bare", sourceRepo); err != nil {
		return fmt.Errorf("failed to create bare clone: %w", err)
	}

	// Get into the bare repository directory
	repoDir := repoToFork + ".git"
	repoName := strings.Split(repoDir, "/")[1]
	defer cleanup(repoName)

	// Create new private repository
	destRepo := determineDestRepo(repoToFork, opts.Organization, opts.ForkName)
	fmt.Printf("Creating private repository %s...\n", destRepo)

	createArgs := []string{"repo", "create", destRepo, "--private"}
	if opts.DefaultBranchOnly {
		createArgs = append(createArgs, "--default-branch-only")
	}

	_, stderr, err := gh.Exec(createArgs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", stderr.String())
		return fmt.Errorf("failed to create private repository: %w", err)
	}

	// Push to new private repository
	fmt.Println("Pushing to private repository...")
	pushCmd := exec.Command("git", "-C", repoName, "push", "--mirror", fmt.Sprintf("git@github.com:%s.git", destRepo))
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("failed to push to private repository: %w", err)
	}

	// Handle cloning if requested
	if opts.Clone {
		err = handleClone(destRepo, repoToFork, opts)
		if err != nil {
			return err
		}
	}

	// Handle remote if requested
	if opts.Remote {
		err = handleRemote(destRepo, opts.RemoteName)
		if err != nil {
			return err
		}
	}

	fmt.Printf("✓ Created private fork %s\n", destRepo)
	return nil
}

func handleClone(destRepo, sourceRepo string, opts *PrivateForkOptions) error {
	fmt.Printf("Cloning fork %s...\n", destRepo)
	cloneURL := fmt.Sprintf("git@github.com:%s.git", destRepo)

	args := append([]string{"clone"}, opts.GitArgs...)
	args = append(args, cloneURL)

	if err := execGit(args...); err != nil {
		return fmt.Errorf("failed to clone: %w", err)
	}

	// Add upstream remote
	repoName := strings.Split(destRepo, "/")[1]
	upstreamURL := fmt.Sprintf("https://github.com/%s.git", sourceRepo)

	args = []string{"-C", repoName, "remote", "add", "upstream", upstreamURL}
	if err := execGit(args...); err != nil {
		return fmt.Errorf("failed to add upstream remote: %w", err)
	}

	return nil
}

func handleRemote(destRepo, remoteName string) error {
	// Check if remote exists
	output, err := exec.Command("git", "remote").Output()
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, remote := range remotes {
		if remote == remoteName {
			// Rename existing remote to upstream
			if err := execGit("remote", "rename", remoteName, "upstream"); err != nil {
				return fmt.Errorf("failed to rename remote: %w", err)
			}
			break
		}
	}

	// Add new remote
	if err := execGit("remote", "add", remoteName, fmt.Sprintf("https://github.com/%s.git", destRepo)); err != nil {
		return fmt.Errorf("failed to add remote: %w", err)
	}

	return nil
}

func parseRepository(repo string) (string, error) {
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		u, err := url.Parse(repo)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid repository URL")
		}
		return fmt.Sprintf("%s/%s", parts[0], parts[1]), nil
	}

	if strings.HasPrefix(repo, "git@") {
		parts := strings.Split(strings.Split(repo, ":")[1], "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid repository SSH URL")
		}
		return fmt.Sprintf("%s/%s", parts[0], strings.TrimSuffix(parts[1], ".git")), nil
	}

	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repository format. Use OWNER/REPO")
	}
	return repo, nil
}

func determineDestRepo(sourceRepo, org, forkName string) string {
	parts := strings.Split(sourceRepo, "/")
	repoName := parts[1]

	if forkName != "" {
		repoName = forkName
	}

	if org != "" {
		return fmt.Sprintf("%s/%s", org, repoName)
	}

	// Get current user
	stdout, stderr, err := gh.Exec("api", "user", "--jq", ".login")
	if err != nil {
		// Fallback to getting it from git config
		stdout, stderr, err = gh.Exec("config", "get", "github.user")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", stderr.String())
			return fmt.Sprintf("OWNER/%s", repoName)
		}
	}

	return fmt.Sprintf("%s/%s", strings.TrimSpace(stdout.String()), repoName)
}

func cleanup(repoDir string) {
	if err := os.RemoveAll(repoDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to clean up temporary directory %s: %v\n", repoDir, err)
	}
}
