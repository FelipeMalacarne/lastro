package fixtures

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Anonymizer rewrites Pluggy payloads so they can be committed as fixtures.
//
// Identifiers, names, documents and account numbers are replaced by
// pseudonyms derived from a random key, so the same value always becomes the
// same pseudonym within one run (an account id still matches its
// transactions, a payer document is still the same person) but nothing can be
// traced back once the key is gone. Every amount is multiplied by one random
// factor, which keeps the relations the tests rely on (a transfer out equals
// the transfer in, a bill payment equals the bill) without keeping the
// values. Dates, statuses, types and banking vocabulary are kept.
type Anonymizer struct {
	key    []byte
	factor *big.Rat
}

// NewAnonymizer creates an anonymizer with a fresh random key and a random
// amount factor between 0.60 and 1.40.
func NewAnonymizer() (*Anonymizer, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(81))
	if err != nil {
		return nil, err
	}
	return newAnonymizer(key, 60+n.Int64()), nil
}

func newAnonymizer(key []byte, factorPercent int64) *Anonymizer {
	return &Anonymizer{key: key, factor: big.NewRat(factorPercent, 100)}
}

// Dir anonymizes every file of a dump into dst, keeping the layout. File
// names are ids, so they are pseudonymized too.
func (a *Anonymizer) Dir(src, dst string) error {
	for _, kind := range []string{"items", "accounts", "transactions", "investments"} {
		err := eachFile(src, kind, func(name string, b []byte) error {
			out, err := a.JSON(b)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(dst, kind), 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dst, kind, a.pseudo(name)+".json"), out, 0o600)
		})
		if err != nil {
			return fmt.Errorf("fixtures: anonymize: %w", err)
		}
	}
	return nil
}

// JSON anonymizes one document.
func (a *Anonymizer) JSON(b []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(a.walk("", "", v), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Keys whose values are amounts of money (or quantities that reveal them).
var amountKeys = set("amount", "amountInAccountCurrency", "balance", "totalAmount", "creditLimit",
	"availableCreditLimit", "minimumPayment", "minimumPaymentAmount", "balanceForeignCurrency",
	"closingBalance", "automaticallyInvestedBalance", "overdraftContractedLimit", "overdraftUsedLimit",
	"unarrangedOverdraftAmount", "quantity", "amountOriginal", "amountProfit", "amountWithdrawal",
	"taxes", "taxes2", "baseAmount", "interestAmount", "penaltyAmount", "discountAmount", "usedAmount",
	"limitAmount", "customizedLimitAmount", "availableAmount")

// Keys whose values identify an account, a document or a record.
var idKeys = set("id", "itemId", "accountId", "billId", "providerId", "providerCode", "referenceNumber",
	"receiverReferenceId", "transferNumber", "number", "cardNumber", "accountNumber", "branchNumber",
	"identificationNumber", "digitableLine", "barcode", "taxNumber", "cnpj", "cpf", "value",
	"proposalNumber", "processNumber", "clientUserId", "issuerCNPJ")

// Keys whose values are a person's or a company's name.
var nameKeys = set("owner", "businessName")

// Objects that describe a counterpart: every name inside them is a person or a company.
var partyKeys = set("payer", "receiver", "merchant", "insurer", "debtor", "documentNumber")

var textKeys = set("description", "descriptionRaw", "reason", "providerMessage")

// Keys whose values are URLs of the owner's own services.
var urlKeys = set("webhookUrl", "oauthRedirectUri")

// walk rewrites v. parent is the key of the enclosing object, key the key of v.
func (a *Anonymizer) walk(parent, key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = a.walk(key, k, child)
		}
		return x
	case []any:
		for i, child := range x {
			x[i] = a.walk(parent, key, child)
		}
		return x
	case json.Number:
		if amountKeys[key] {
			return a.scale(x)
		}
		return x
	case string:
		switch {
		case x == "":
			return x
		case key == "value" && parent != "documentNumber":
			return x // e.g. a currency or a code value, not a document
		case idKeys[key]:
			return a.pseudo(x)
		case nameKeys[key], key == "name" && partyKeys[parent]:
			return a.pseudoText(x, false)
		case textKeys[key]:
			return a.pseudoText(x, true)
		case urlKeys[key]:
			return "https://example.com/hook"
		case emailRe.MatchString(x):
			return "person@example.com"
		}
		// Any other string (dates, enums, product names) is kept, except a
		// CPF or CNPJ hiding inside it.
		return documentRe.ReplaceAllStringFunc(x, a.pseudo)
	}
	return v
}

// scale multiplies an amount by the factor, rounding half away from zero to
// cents (to six places for quantities, which can be fractional).
func (a *Anonymizer) scale(n json.Number) json.Number {
	r, ok := new(big.Rat).SetString(n.String())
	if !ok {
		return n
	}
	r.Mul(r, a.factor)
	places := 2
	if strings.Contains(n.String(), ".") && len(n.String())-strings.Index(n.String(), ".")-1 > 2 {
		places = 6
	}
	s := r.FloatString(places)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return json.Number(s)
}

var (
	uuidRe  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	emailRe = regexp.MustCompile(`[^\s@]+@[^\s@]+\.[^\s@]+`)
	// CPF and CNPJ, formatted or bare, wherever they appear inside free text.
	documentRe = regexp.MustCompile(`\d{3}\.\d{3}\.\d{3}-\d{2}|\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}|\b\d{11}\b|\b\d{14}\b`)
)

// pseudo replaces an identifier by another of the same shape: a UUID stays a
// UUID, digits stay digits, letters stay letters, separators are kept.
func (a *Anonymizer) pseudo(s string) string {
	h := a.stream(s)
	if uuidRe.MatchString(s) {
		b := h.bytes(16)
		b[6] = b[6]&0x0f | 0x40
		b[8] = b[8]&0x3f | 0x80
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	var out strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			out.WriteByte('0' + h.next()%10)
		case r >= 'a' && r <= 'z':
			out.WriteByte('a' + h.next()%26)
		case r >= 'A' && r <= 'Z':
			out.WriteByte('A' + h.next()%26)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// pseudoText replaces every word that is not banking vocabulary with a
// pronounceable pseudoword, the same one each time the word appears. With
// keepVocabulary false (names) every word is replaced.
func (a *Anonymizer) pseudoText(s string, keepVocabulary bool) string {
	s = documentRe.ReplaceAllStringFunc(s, a.pseudo)
	var out strings.Builder
	word := []rune{}
	flush := func() {
		if len(word) == 0 {
			return
		}
		w := string(word)
		switch {
		case isDigits(w) && len(w) < 4, keepVocabulary && vocabulary[fold(w)]:
			out.WriteString(w)
		case isDigits(w):
			out.WriteString(a.pseudo(w))
		default:
			out.WriteString(matchCase(w, a.pseudoword(fold(w), len(word))))
		}
		word = word[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word = append(word, r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

var syllables = []string{"ba", "ca", "da", "fe", "ga", "le", "ma", "no", "pa", "ri", "sa", "to", "va", "ze", "lu", "mi", "ro", "ti", "na", "be"}

func (a *Anonymizer) pseudoword(w string, n int) string {
	h := a.stream("word:" + w)
	count := max(2, min(4, (n+1)/2))
	var b strings.Builder
	for range count {
		b.WriteString(syllables[int(h.next())%len(syllables)])
	}
	return b.String()
}

// stream is a deterministic byte stream keyed by the anonymizer key and a value.
type stream struct {
	block []byte
	pos   int
	mac   func(counter uint32) []byte
}

func (a *Anonymizer) stream(s string) *stream {
	st := &stream{}
	st.mac = func(counter uint32) []byte {
		m := hmac.New(sha256.New, a.key)
		_ = binary.Write(m, binary.BigEndian, counter)
		m.Write([]byte(s))
		return m.Sum(nil)
	}
	return st
}

func (s *stream) next() byte {
	if s.pos%32 == 0 {
		s.block = s.mac(uint32(s.pos / 32))
	}
	b := s.block[s.pos%32]
	s.pos++
	return b
}

func (s *stream) bytes(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = s.next()
	}
	return out
}

func matchCase(original, w string) string {
	switch {
	case strings.ToUpper(original) == original:
		return strings.ToUpper(w)
	case unicode.IsUpper([]rune(original)[0]):
		return strings.ToUpper(w[:1]) + w[1:]
	}
	return w
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// fold upper-cases and strips accents, so "Cartão" and "CARTAO" are one word.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

func set(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// vocabulary is what banks write in descriptions, plus institutions. These
// words describe the kind of movement, never who it was with.
var vocabulary = set(
	"PIX", "TED", "DOC", "TEV", "TRANSF", "TRANSFERENCIA", "ENVIADO", "ENVIADA", "ENVIO", "RECEBIDO",
	"RECEBIDA", "REALIZADO", "REALIZADA", "PAGAMENTO", "PAGTO", "PGTO", "PAG", "FATURA", "CARTAO",
	"CREDITO", "DEBITO", "COMPRA", "COMPRAS", "SAQUE", "TARIFA", "TARIFAS", "RENDIMENTO", "RENDIMENTOS",
	"RESGATE", "APLICACAO", "APLIC", "ESTORNO", "BOLETO", "IOF", "JUROS", "MULTA", "PARCELA", "PARCELADO",
	"PARCELADA", "SALARIO", "DEPOSITO", "CONTA", "CORRENTE", "POUPANCA", "AUTOMATICA", "AUTOMATICO",
	"AUT", "INT", "INTERNACIONAL", "NACIONAL", "ANUIDADE", "MENSALIDADE", "ASSINATURA", "CASHBACK",
	"RECARGA", "CELULAR", "CONVENIO", "ARRECADACAO", "FOLHA", "PORTABILIDADE", "ENCARGOS", "CHEQUE",
	"ESPECIAL", "LIMITE", "INVESTIMENTO", "INVESTIMENTOS", "CDB", "RDB", "LCI", "LCA", "TESOURO",
	"DIRETO", "FUNDO", "FUNDOS", "CAIXINHA", "COFRINHO", "RESERVA", "AGENDADO", "AGENDAMENTO", "DEVOLUCAO",
	"DEVOLVIDO", "EXTERIOR", "SEGURO", "EMPRESTIMO", "FINANCIAMENTO", "CREDITADO", "DEBITADO", "VALOR",
	"ENTRE", "MESMA", "TITULARIDADE", "TITULAR", "PROPRIA", "OUTRA", "OUTRAS", "INSTITUICAO", "BANCO",
	"NUBANK", "NU", "ITAU", "BRADESCO", "SANTANDER", "INTER", "XP", "BTG", "PACTUAL", "CAIXA", "C6",
	"BB", "SICOOB", "SICREDI", "MERCADO", "PAGO", "PICPAY", "PAGBANK", "NEON", "ORIGINAL", "RICO",
	"CLEAR", "MODAL", "GENIAL", "AVENUE", "NOMAD", "WISE",
	"DE", "DA", "DO", "DAS", "DOS", "EM", "PARA", "E", "A", "O", "AO", "NO", "NA", "POR", "COM",
	"S", "SA", "LTDA", "ME", "EIRELI",
)
