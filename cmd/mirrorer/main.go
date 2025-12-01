package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/sourcehawk/operator-api-mirrorer/pkg"
	"github.com/spf13/cobra"
)

// flag vars scoped at package level so both parsing and RunE can access them.
// Each command defines its own flags; root has none.
var (
	mirrorConfigPath  string
	mirrorRootPath    string
	mirrorGitRepo     string
	mirrorTarget      string
	tagConfigPath     string
	tagMirrorRootPath string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mirrorer",
		Short: "Generate and manage mirrored operator API modules",
	}

	/*
		------------------------------------------------------------
		  mirror command
		------------------------------------------------------------
	*/
	mirrorCmd := &cobra.Command{
		Use:   "mirror",
		Short: "Generate/update mirrored operator APIs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mirrorGitRepo == "" {
				return fmt.Errorf("--gitRepo is required")
			}

			operators, err := readOperatorsFile(mirrorConfigPath)
			if err != nil {
				return err
			}

			return runMirror(operators, mirrorRootPath, mirrorGitRepo, mirrorTarget)
		},
	}
	mirrorCmd.Flags().StringVar(&mirrorConfigPath, "config", "operators.yaml", "path to operators.yaml")
	mirrorCmd.Flags().StringVar(&mirrorRootPath, "mirrorsPath", "mirrors", "path to mirroring root directory")
	mirrorCmd.Flags().StringVar(&mirrorGitRepo, "gitRepo", "", "git repo root (module path)")
	mirrorCmd.Flags().StringVar(&mirrorTarget, "target", "", "optional target (slug) from config file")

	/*
		------------------------------------------------------------
		  tag command
		------------------------------------------------------------
	*/
	tagCmd := &cobra.Command{
		Use:   "tag",
		Short: "Create git tags for current operator versions defined in operators.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			operators, err := readOperatorsFile(tagConfigPath)
			if err != nil {
				return err
			}

			return runTagging(operators, tagMirrorRootPath)
		},
	}
	tagCmd.Flags().StringVar(&tagConfigPath, "config", "operators.yaml", "path to operators.yaml")
	tagCmd.Flags().StringVar(&tagMirrorRootPath, "mirrorsPath", "mirrors", "path to mirroring root directory")

	rootCmd.AddCommand(mirrorCmd, tagCmd)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	// no default action: user must pick a subcommand
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

/*
------------------------------------------------------------
  RUN HELPERS
------------------------------------------------------------
*/

func runMirror(operators *pkg.OperatorsFile, mirrorRootPath, gitRepo, mirrorTarget string) error {
	if mirrorTarget != "" {
		log.Printf("Mirroring for target: %s", mirrorTarget)
	}

	log.Printf("Operators config: %+v", operators)

	if err := operators.Process(mirrorRootPath, gitRepo, mirrorTarget); err != nil {
		return err
	}

	log.Printf("Mirror generation completed successfully.")
	return nil
}

func runTagging(operators *pkg.OperatorsFile, mirrorRootPath string) error {
	created, err := operators.Tag(mirrorRootPath)
	if err != nil {
		return err
	}
	log.Printf("Tagging completed successfully.")
	if created == 0 {
		log.Printf("No new tags were created")
	} else {
		log.Printf("Please note that you must push the tags yourself.")
		log.Printf("You can do this with: git push origin --tags")
	}

	return nil
}

/*
------------------------------------------------------------
  CONFIG LOADING
------------------------------------------------------------
*/

func readOperatorsFile(path string) (*pkg.OperatorsFile, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(root, path)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	config := &pkg.OperatorsFile{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}
