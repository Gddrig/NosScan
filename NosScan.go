package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/enescakir/emoji"
	"github.com/fatih/color"
	"github.com/go-resty/resty/v2"
	"github.com/rodaine/table"
)

type ConfigEntry struct {
	Account      string `json:"account"`
	TargetPubkey string `json:"targetPubkey"`
	Name         string `json:"name"`
}

type ConfigEntries []ConfigEntry

type TokenAccounts struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		Context struct {
			Slot int `json:"slot"`
		} `json:"context"`
		Value []struct {
			Account struct {
				Data struct {
					Parsed struct {
						Info struct {
							TokenAmount struct {
								Amount   string `json:"amount"`
								Decimals int    `json:"decimals"`
							} `json:"tokenAmount"`
						} `json:"info"`
					} `json:"parsed"`
				} `json:"data"`
			} `json:"account"`
			Pubkey string `json:"pubkey"`
		} `json:"value"`
	} `json:"result"`
	Id int `json:"id"`
}

type SolanaBalance struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		Value uint64 `json:"value"`
	} `json:"result"`
	Id int `json:"id"`
}

type Signatures struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  []struct {
		Signature          string      `json:"signature"`
		Slot               int         `json:"slot"`
		Err                interface{} `json:"err"`
		Memo               interface{} `json:"memo"`
		BlockTime          int         `json:"blockTime"`
		ConfirmationStatus string      `json:"confirmationStatus"`
	} `json:"result"`
	Id int `json:"id"`
}

type Widget struct {
	ID        int
	Name      string
	Account   string
	Solana    float64
	Balance   float64
	LastEntry time.Time
	Difftime  time.Duration
	Active    string // Nouvelle colonne pour indiquer l'activité
}

var tbl table.Table
var nextID int // Variable pour stocker le prochain ID

func main() {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	cyan := color.New(color.FgCyan)
	//magenta := color.New(color.FgMagenta)
	yellow := color.New(color.FgYellow)

	// Initialiser l'ID unique
	nextID = 1

	// Lire les données de configuration depuis le fichier JSON
	configFile, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var configEntries ConfigEntries
	err = json.Unmarshal(configFile, &configEntries)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}
	
	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	// Initialiser le tableau avec les en-têtes
	tbl = table.New("ID", "Name", "Account", "Solana", "NOS", "Last Entry", "Difftime", "Active")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)

	fmt.Println("")

	for _, entry := range configEntries {
		cyan.Printf("Account: %s, Name: %s\n", entry.Account, entry.Name)

		tokenAccounts, err := getTokenAccountsByOwner(entry.Account)
		if err != nil {
			log.Printf("Error getting token accounts for %s: %v\n", entry.Name, err)
			continue // Passer au prochain compte en cas d'erreur
		}

		var nosBalance float64
		found := false
		for _, value := range tokenAccounts.Result.Value {
			if value.Pubkey == entry.TargetPubkey {
				nosBalance, err = strconv.ParseFloat(value.Account.Data.Parsed.Info.TokenAmount.Amount, 64)
				if err != nil {
					log.Printf("Error parsing amount for %s: %v\n", entry.Name, err)
					continue // Passer au prochain compte en cas d'erreur
				}
				decimals := value.Account.Data.Parsed.Info.TokenAmount.Decimals
				nosBalance = nosBalance / math.Pow(10, float64(decimals))
				green.Printf("NOS Balance: %.2f NOS\n", nosBalance)
				found = true
				break
			}
		}

		if !found {
			fmt.Printf("No NOS balance found for %s with pubkey %s\n", entry.Name, entry.TargetPubkey)
		}

		solanaBalance, err := getSolanaBalance(entry.Account)
		if err != nil {
			log.Printf("Error getting Solana balance for %s: %v\n", entry.Name, err)
			continue // Passer au prochain compte en cas d'erreur
		}

		signatures, err := getConfirmedSignaturesForAddress2(entry.Account)
		if err != nil {
			log.Printf("Error getting confirmed signatures for %s: %v\n", entry.Name, err)
			continue // Passer au prochain compte en cas d'erreur
		}

		if len(signatures.Result) > 0 {
			lastSignature := signatures.Result[0]
			if lastSignature.BlockTime != 0 {
				blockTime := time.Unix(int64(lastSignature.BlockTime), 0)
				now := time.Now()
				difftime := now.Sub(blockTime)

				var difftimeColor *color.Color
				var activeStatus string
				if difftime.Seconds() > 3*3600 { // 3 heures en secondes
					difftimeColor = red
					activeStatus = emoji.RedCircle.String()
				} else {
					difftimeColor = green
					activeStatus = emoji.GreenCircle.String()
				}

				difftimeColor.Printf("Last Confirmed Signature: %s (%s)\n\n", blockTime.Format("2006/01/02 15:04:05"), difftime.Round(time.Second))

				// Ajouter une ligne au tableau avec les données formatées et l'ID incrémenté
				addRowToTable(Widget{
					ID:        nextID,
					Name:      entry.Name,
					Account:   shortenString(entry.Account, 10, 10),
					Solana:    solanaBalance,
					Balance:   nosBalance,
					LastEntry: blockTime,
					Difftime:  difftime,
					Active:    activeStatus, // Définir le statut actif
				})

				// Incrémenter l'ID pour le prochain widget
				nextID++
			}
		} else {
			fmt.Printf("No confirmed signatures found for %s\n\n", entry.Name)
		}

		time.Sleep(5 * time.Second) // Attendre 5 secondes avant de passer au prochain compte
	}

	// Afficher le tableau à la fin
	tbl.Print()
	now2 := time.Now()
	yellow.Printf("\nLast Update : %s", now2.Format("2006/01/02 15:04:05"))
	fmt.Println("\n")
}

func addRowToTable(widget Widget) {
	var redgreen string
	if widget.Solana < 0.025 {
		redgreen = emoji.RedCircle.String()
		tbl.AddRow(widget.ID, widget.Name, widget.Account, fmt.Sprintf("%.4f SOL%s", widget.Solana, redgreen), fmt.Sprintf("%.2f NOS", widget.Balance), widget.LastEntry.Format("2006/01/02 15:04:05"), widget.Difftime.Round(time.Second), widget.Active)
	} else {
		redgreen = emoji.GreenCircle.String()
		tbl.AddRow(widget.ID, widget.Name, widget.Account, fmt.Sprintf("%.4f SOL%s", widget.Solana, redgreen), fmt.Sprintf("%.2f NOS", widget.Balance), widget.LastEntry.Format("2006/01/02 15:04:05"), widget.Difftime.Round(time.Second), widget.Active)
	}
}

func getTokenAccountsByOwner(owner string) (*TokenAccounts, error) {
	client := resty.New()
	url := "https://api.mainnet-beta.solana.com"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTokenAccountsByOwner",
		"params": []interface{}{
			owner,
			map[string]string{"programId": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"},
			map[string]string{"encoding": "jsonParsed"},
		},
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(url)

	if err != nil {
		return nil, err
	}

	var tokenAccounts TokenAccounts
	err = json.Unmarshal(resp.Body(), &tokenAccounts)
	if err != nil {
		return nil, err
	}

	return &tokenAccounts, nil
}

func getSolanaBalance(account string) (float64, error) {
	client := resty.New()
	url := "https://api.mainnet-beta.solana.com"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBalance",
		"params":  []interface{}{account},
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(url)

	if err != nil {
		return 0, err
	}

	var solanaBalance SolanaBalance
	err = json.Unmarshal(resp.Body(), &solanaBalance)
	if err != nil {
		return 0, err
	}

	// Solana balance is returned in lamports (1 SOL = 1e9 lamports)
	return float64(solanaBalance.Result.Value) / 1e9, nil
}

func getConfirmedSignaturesForAddress2(account string) (*Signatures, error) {
	client := resty.New()
	url := "https://api.mainnet-beta.solana.com"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getConfirmedSignaturesForAddress2",
		"params": []interface{}{
			account,
			map[string]interface{}{
				"limit": 1,
			},
		},
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(url)

	if err != nil {
		return nil, err
	}

	var signatures Signatures
	err = json.Unmarshal(resp.Body(), &signatures)
	if err != nil {
		return nil, err
	}

	return &signatures, nil
}

func shortenString(str string, startLen int, endLen int) string {
	if len(str) <= startLen+endLen {
		return str
	}
	return str[:startLen] + "..." + str[len(str)-endLen:]
}