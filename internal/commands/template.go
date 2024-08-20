package commands

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/spf13/cobra"
	"google.golang.org/api/iterator"
)

var templates = make(map[string]string)

const (
	templateFilePath = "/.config/gemini-cli-templates"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"t"},
	Short:   "Send a prompt with templates",
	Long:    strings.TrimSpace(templateUsage),
	Run:     runTemplateCmd,
}

var templateUsage = `
Use template to generate prompts, and send that to the model.

The template is a string with placeholders for the user input, 
for example, "translate %s to english, and give me detailed explanations".
You can add a template with "-a key", and use it with "-u key".
The text args will be inserted into the template.

Except for the template part of this command, 
the other usages are the same as "prompt" command.

You can also edit /.config/gemini-cli-templates in your home directory directly,
just add a line with "key:value" format.

`

func init() {
	rootCmd.AddCommand(templateCmd)

	templateCmd.Flags().StringP("add", "a", "", "add a template with a key")
	templateCmd.Flags().StringP("use", "u", "", "use a template")
	templateCmd.Flags().Bool("stream", true, "stream the response from the model")
	templateCmd.Flags().String("temp", "", "temperature setting for the model")
	templateCmd.Flags().BoolP("list", "l", false, "list templates")
	//read config to get templates
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile(homeDir+templateFilePath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, ":")
		if ok {
			key := strings.TrimSpace(key)
			value := strings.TrimSpace(value)
			templates[key] = value
		}
	}
}

func runTemplateCmd(cmd *cobra.Command, args []string) {
	if mustGetBoolFlag(cmd, "list") {
		for key, value := range templates {
			fmt.Printf("%s\t:%s\n", key, value)
		}
		return
	}

	addKey := mustGetStringFlag(cmd, "add")
	if addKey != "" && len(args) == 1 {
		addTemplate(addKey, args[0])
		return
	}

	//if don't use template, run prompt mode
	useKey := mustGetStringFlag(cmd, "use")
	if useKey == "" {
		cmd.Flags().String("system", "", "")
		runPromptCmd(cmd, args)
	} else {
		promptParts := []genai.Part{}
		template := templates[useKey]
		textPrompt := []string{}

		for _, arg := range args {
			if argLooksLikeURL(arg) {
				part, err := getPartFromURL(arg)
				if err != nil {
					log.Fatal(err)
				}
				promptParts = append(promptParts, part)
			} else if argLooksLikeFilename(arg) {
				part, err := getPartFromFile(arg)
				if err != nil {
					log.Fatal(err)
				}
				promptParts = append(promptParts, part)
			} else {
				textPrompt = append(textPrompt, arg)
			}
		}

		placeholdersCnt := strings.Count(template, "%s")
		for _, text := range textPrompt {
			if placeholdersCnt > 0 {
				template = strings.Replace(template, "%s", text, 1)
				placeholdersCnt--
			} else {
				break
			}
		}
		promptParts = append(promptParts, genai.Text(template))

		ctx := context.Background()
		client, err := newGenaiClient(ctx, cmd)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		model := client.GenerativeModel(mustGetStringFlag(cmd, "model"))

		if tempValue := mustGetStringFlag(cmd, "temp"); tempValue != "" {
			f, err := strconv.ParseFloat(tempValue, 32)
			if err != nil {
				log.Fatalf("problem parsing --temp value: %v", err)
			}
			model.SetTemperature(float32(f))
		}

		model.SafetySettings = []*genai.SafetySetting{
			{
				Category:  genai.HarmCategoryDangerousContent,
				Threshold: genai.HarmBlockNone,
			},
			{
				Category:  genai.HarmCategoryHarassment,
				Threshold: genai.HarmBlockNone,
			},
			{
				Category:  genai.HarmCategoryHateSpeech,
				Threshold: genai.HarmBlockNone,
			},
			{
				Category:  genai.HarmCategorySexuallyExplicit,
				Threshold: genai.HarmBlockNone,
			},
		}

		if stream := mustGetBoolFlag(cmd, "stream"); stream {
			iter := model.GenerateContentStream(ctx, promptParts...)
			for {
				resp, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Fatal(err)
				}
				if len(resp.Candidates) < 1 {
					fmt.Println("<empty response from model>")
				} else {
					c := resp.Candidates[0]
					if c.Content != nil {
						for _, part := range c.Content.Parts {
							fmt.Print(part)
						}
					} else {
						fmt.Println("<empty response from model>")
					}
				}
			}
			fmt.Println()
		} else {
			resp, err := model.GenerateContent(ctx, promptParts...)
			if err != nil {
				log.Fatal(err)
			}
			if len(resp.Candidates) < 1 {
				fmt.Println("<empty response from model>")
			} else {
				c := resp.Candidates[0]
				if c.Content != nil {
					for _, part := range c.Content.Parts {
						fmt.Println(part)
					}
				} else {
					fmt.Println("<empty response from model>")
				}
			}
		}
	}
}

func addTemplate(key string, value string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile(homeDir+templateFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	_, err = writer.WriteString(fmt.Sprintf("%s:%s\n", key, value))
	if err != nil {
		log.Fatal("problem happened while writing to file:", err)
	}
}
