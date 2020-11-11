package commands

import (
	"fmt"
	"strconv"

	"github.com/buger/goterm"
	"github.com/jfrog/jfrog-cli-core/artifactory/commands"
	"github.com/jfrog/jfrog-cli-core/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-core/artifactory/spec"
	"github.com/jfrog/jfrog-cli-core/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/plugins/components"
	"github.com/jfrog/jfrog-cli-core/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

func GetLsCommand() components.Command {
	return components.Command{
		Name:        "ls",
		Description: "Run ls.",
		Aliases:     []string{"ls, list"},
		Arguments:   getLsArguments(),
		Flags:       getLsFlags(),
		Action: func(c *components.Context) error {
			return lsCmd(c)
		},
	}
}

func getLsArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "[Mandatory] Path in Artifactory.",
		},
	}
}

func getLsFlags() []components.Flag {
	return []components.Flag{
		components.StringFlag{
			Name:        "server-id",
			Description: "Artifactory server ID configured using the config command.",
		},
	}
}

type lsConfiguration struct {
	details *config.ArtifactoryDetails
	path    string
}

func lsCmd(c *components.Context) error {
	if err := checkInputs(c); err != nil {
		return err
	}

	confDetails, err := getRtDetails(c)
	if err != nil {
		return err
	}

	// Increase log level to avoid search command logs
	increaseLogLevel()

	conf := &lsConfiguration{
		details: confDetails,
		path:    c.Arguments[0],
	}

	return doLs(conf)
}

func doLs(c *lsConfiguration) error {
	// Execute search command
	reader, err := doSearch(c)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Get structured search results and the path with the max length
	searchResults, maxPathLength, err := processSearchResults(c.path, reader)
	if err != nil {
		return err
	}

	// Print results
	printLsResults(searchResults, maxPathLength)

	return nil
}

func doSearch(c *lsConfiguration) (*content.ContentReader, error) {
	// Run the first search
	searchCmd := generic.NewSearchCommand()
	searchSpec := spec.NewBuilder().Pattern(c.path).IncludeDirs(true).BuildSpec()
	searchCmd.SetRtDetails(c.details).SetSpec(searchSpec)
	if err := commands.Exec(searchCmd); err != nil {
		return nil, err
	}

	// Check the search results
	reader := searchCmd.Result().Reader()
	if err := checkSearchResults(reader, c.path); err != nil {
		reader.Close()
		return nil, err
	}

	// Check if a second search is needed
	runSecondSearch, err := shouldRunSecondSearch(c.path, reader)
	if !runSecondSearch || err != nil {
		return reader, err
	}

	// Close the first search reader
	if err := reader.Close(); err != nil {
		return nil, err
	}

	// Run search again with "/" in the end of the pattern
	searchSpec = spec.NewBuilder().Pattern(c.path + "/").IncludeDirs(true).BuildSpec()
	searchCmd.SetSpec(searchSpec)
	err = commands.Exec(searchCmd)
	return searchCmd.Result().Reader(), err
}

func printLsResults(searchResults []utils.SearchResult, maxPathLength int) {
	maxPathLength += minSpace
	maxResultsInLine := goterm.Width() / maxPathLength
	if maxResultsInLine == 0 {
		maxResultsInLine = 1
	}

	pattern := "%-" + strconv.Itoa(maxPathLength) + "s"
	var color int
	for i, res := range searchResults {
		if i > 0 && i%maxResultsInLine == 0 {
			fmt.Println()
		}
		if res.Type == "folder" {
			color = goterm.BLUE
		} else {
			color = goterm.WHITE
		}
		output := fmt.Sprintf(pattern, res.Path)
		fmt.Print(goterm.Color(output, color))
	}
	fmt.Println()
}

// Gets the search results and builds an array of SearchResults.
// Return also the path with the maximum size.
func processSearchResults(pattern string, reader *content.ContentReader) ([]utils.SearchResult, int, error) {
	if err := checkSearchResults(reader, pattern); err != nil {
		return nil, 0, err
	}

	allResults := []utils.SearchResult{}
	maxPathLength := 0
	result := new(utils.SearchResult)
	for i := 0; reader.NextRecord(result) == nil; i++ {
		result.Path = trimFoldersFromPath(pattern, result.Path)
		pathLength := len(result.Path)
		if pathLength > 0 {
			if pathLength > maxPathLength {
				maxPathLength = pathLength
			}
			allResults = append(allResults, *result)
		}
		result = new(utils.SearchResult)
	}
	return allResults, maxPathLength, reader.GetError()
}
