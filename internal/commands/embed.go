package commands

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/eliben/gemini-cli/internal/apikey"
	"github.com/google/generative-ai-go/genai"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Embed an input using an embedding model",
	Long:  `Use a Gemini embedding model to embed a single string of content`,
	Run:   runEmbedCmd,
}

func init() {
	rootCmd.AddCommand(embedCmd)

	embedCmd.Flags().StringP("content", "c", "", "content to embed")
	embedCmd.Flags().StringP("model", "m", "embedding-001", "name of embedding model to use")
	embedCmd.Flags().String("format", "json", "format for embedding output: json, base64, blob")
	embedCmd.Flags().String("db", "", "DB file to write embeddings into")
	embedCmd.Flags().String("table", "embeddings", "DB table name to store embeddings into")
	embedCmd.Flags().String("sql", "", "SQL mode with a query")
	embedCmd.Flags().StringSlice("attach", nil, "additional DB to attach - specify <alias>,<filename> pair")
}

// TODO: API with SQLite
// --db specifies output DB: in this case the output is written into this
// DB, not stdout
// in DB, ID should be string, to incorporate arbitrary IDs not just numeric,
// especially with input files
// then input is either taken as auto-deteecting file (passed as arg or piped
// into stdin with -), or the DB itself with --sql flag. --attach also works.
// --files will take input from file system dir
// --table specifies which table to write results to
// maybe --format should be repurposed for input file format?
// output will always be JSON to stdout, or blob to DB
//

func runEmbedCmd(cmd *cobra.Command, args []string) {
	if dbPath := mustGetStringFlag(cmd, "db"); dbPath != "" {
		embedModeDB(cmd, args, dbPath)
	} else if content := mustGetStringFlag(cmd, "content"); content != "" {
		embedModeContent(cmd, args, content)
	} else {
		log.Fatal("expect either --db or --content")
	}
}

// embedModeContent runs the --content mode of the embed command.
func embedModeContent(cmd *cobra.Command, args []string, content string) {
	key := apikey.Get(cmd)

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(key))
	if err != nil {
		log.Fatal()
	}

	model := client.EmbeddingModel(mustGetStringFlag(cmd, "model"))
	res, err := model.EmbedContent(ctx, genai.Text(content))

	if emb := res.Embedding; emb != nil {
		emitEmbedding(os.Stdout, emb.Values, mustGetStringFlag(cmd, "format"))
	} else {
		log.Fatal("got no embedding back from model")
	}
}

// embedModeDB runs the --db mode of the embed command.
func embedModeDB(cmd *cobra.Command, args []string, dbPath string) {
	//key := apikey.Get(cmd)

	sqlMode := mustGetStringFlag(cmd, "sql")

	// TODO: implement input file mode, not just sql
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("unable to open DB at %v", dbPath)
	}
	defer db.Close()

	tableName := mustGetStringFlag(cmd, "table")
	_, err = db.Exec(fmt.Sprintf(`
  CREATE TABLE IF NOT EXISTS %s (
	id TEXT PRIMARY KEY,
	embedding BLOB
	)`, tableName))
	if err != nil {
		log.Fatal("unable to create table in DB: ", tableName)
	}

	// We extract a list of [id, text] pairs - either from the DB itself (in --sql
	// mode) or from an input file. These texts are going to be sent to the model
	// for calculating embeddings. Each text is the concatenation of all the text
	// columns following ID that the SQL query specifies.
	var ids []string
	var texts []string

	if sqlMode != "" {
		attachPair, _ := cmd.Flags().GetStringSlice("attach")
		if len(attachPair) > 0 {
			if len(attachPair) != 2 {
				log.Fatal("expect <alias>,<db path> pair for --attach")
			}

			alias := attachPair[0]
			path := attachPair[1]

			attachStmt := fmt.Sprintf("ATTACH DATABASE '%v' as %v", path, alias)
			_, err := db.Exec(attachStmt)
			if err != nil {
				log.Fatalf("unable to attach %v: %v", path, err)
			}
		}

		rows, err := db.Query(sqlMode)
		if err != nil {
			log.Fatal("error running SQL query:", err)
		}
		defer rows.Close()

		for rows.Next() {
			// Scan all len(colNames) columns into the values slice.
			values := scanRowIntoSlice(rows)
			if len(values) < 2 {
				log.Fatalf("expect at least 2 columns from query; got %v", len(values))
			}

			var rowTexts []string
			for _, v := range values[1:] {
				rowTexts = append(rowTexts, fmt.Sprintf("%v", v))
			}
			ids = append(ids, fmt.Sprintf("%v", values[0]))
			texts = append(texts, strings.Join(rowTexts, " "))
		}

		// Check for errors from iterating over rows.
		if err := rows.Err(); err != nil {
			log.Fatal("error scanning DB:", err)
		}
	} else {

		panic("only sql for now")
	}

	fmt.Println(ids)
	fmt.Println(texts)
	// TODO: now actually embed them

	//ctx := context.Background()
	//client, err := genai.NewClient(ctx, option.WithAPIKey(key))
	//if err != nil {
	//log.Fatal()
	//}

	//modelName, _ := cmd.Flags().GetString("model")
	//model := client.EmbeddingModel(modelName)

}

func emitEmbedding(w io.Writer, v []float32, format string) {
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		err := encoder.Encode(v)
		if err != nil {
			log.Fatal(err)
		}
	case "base64":
		b := encodeEmbedding(v)
		encoder := base64.NewEncoder(base64.StdEncoding, w)
		encoder.Write(b)
		encoder.Close()
		fmt.Println()
	case "blob":
		b := encodeEmbedding(v)
		w.Write(b)
	default:
		log.Fatalf("invalid format: %s", format)
	}
}

// encodeEmbedding encodes an embedding into a byte buffer, e.g. for DB
// storage as a blob.
func encodeEmbedding(emb []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range emb {
		err := binary.Write(buf, binary.LittleEndian, f)
		if err != nil {
			panic(err)
		}
	}
	return buf.Bytes()
}

// decodeEmbedding decodes an embedding back from a byte buffer.
func decodeEmbedding(b []byte) []float32 {
	buf := bytes.NewReader(b)

	// Calculate how many float32 values are in the slice
	count := buf.Len() / 4
	numbers := make([]float32, 0, count)

	for i := 0; i < count; i++ {
		var num float32
		err := binary.Read(buf, binary.LittleEndian, &num)
		if err != nil {
			panic(err)
		}
		numbers = append(numbers, num)
	}
	return numbers
}

// scanRowIntoSlice scans a row into a slice of any.
func scanRowIntoSlice(row *sql.Rows) []any {
	colNames, err := row.Columns()
	if err != nil {
		panic(err)
	}

	values := make([]interface{}, len(colNames))
	scanArgs := make([]interface{}, len(colNames))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	err = row.Scan(scanArgs...)
	if err != nil {
		log.Fatal("error scanning row:", err)
	}
	return values
}
