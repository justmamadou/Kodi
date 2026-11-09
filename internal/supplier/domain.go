// Package supplier gère le répertoire des fournisseurs d'une entreprise : fiche légale et commerciale,
// contacts, conditions commerciales historisées, archivage et contrôle des doublons.
package supplier

import (
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/shopspring/decimal"
)

// Valeurs acceptées, identiques aux contraintes CHECK de la base.
var (
	LegalForms     = []string{"ei", "suarl", "sarl", "sa", "sas", "gie", "other"}
	Activities     = []string{"materials", "equipment_rental", "subcontracting", "transport", "services", "other"}
	PaymentMethods = []string{"cash", "wave", "orange_money", "free_money", "bank_transfer", "cheque", "other"}
)

var (
	phoneRe  = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	nineaRe  = regexp.MustCompile(`^[0-9]{7,9}[0-9A-Z]{0,3}$`)
	rccmRe   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9./ -]{4,39}$`)
	spacesRe = regexp.MustCompile(`\s+`)
)

// Input décrit une fiche fournisseur. En modification, un champ absent reste inchangé et une chaîne
// vide efface un champ facultatif.
type Input struct {
	Name       *string   `json:"name"`
	Phone      *string   `json:"phone"`
	Email      *string   `json:"email"`
	LegalName  *string   `json:"legal_name"`
	LegalForm  *string   `json:"legal_form"`
	NINEA      *string   `json:"ninea"`
	RCCM       *string   `json:"rccm"`
	Address    *string   `json:"address"`
	City       *string   `json:"city"`
	Activities *[]string `json:"activities"`
	Notes      *string   `json:"notes"`
	// ConfirmNotDuplicate passe outre un doublon probable (même téléphone, même email, nom proche),
	// jamais un doublon certain (même nom, NINEA ou RCCM).
	ConfirmNotDuplicate bool `json:"confirm_not_duplicate"`
}

// Fields est une fiche normalisée, prête à être enregistrée.
type Fields struct {
	Name                                            string
	Phone, Email, LegalName, LegalForm, NINEA, RCCM *string
	Address, City, Notes                            *string
	Activities                                      []string
}

// optional retourne nil pour une chaîne vide, la chaîne nettoyée sinon.
func optional(s string) *string {
	s = strings.TrimSpace(spacesRe.ReplaceAllString(s, " "))
	if s == "" {
		return nil
	}
	return &s
}

// NormalizePhone met un numéro au format international : « 77 123 45 67 » → « +221771234567 ».
func NormalizePhone(raw string) string {
	p := strings.NewReplacer(" ", "", ".", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(raw))
	if strings.HasPrefix(p, "00") {
		p = "+" + p[2:]
	}
	// Numéro sénégalais saisi sans indicatif : 9 chiffres commençant par 7 (mobile) ou 3 (fixe).
	if len(p) == 9 && (p[0] == '7' || p[0] == '3') && !strings.ContainsFunc(p, func(r rune) bool { return r < '0' || r > '9' }) {
		p = "+221" + p
	}
	return p
}

func normalizeEmail(v *apperr.Validator, field, raw string) *string {
	e := optional(strings.ToLower(raw))
	if e != nil {
		_, err := mail.ParseAddress(*e)
		v.Check(err == nil && !strings.ContainsAny(*e, "\r\n "), field, "Adresse email invalide.")
	}
	return e
}

func normalizePhoneField(v *apperr.Validator, field, raw string) *string {
	if optional(raw) == nil {
		return nil
	}
	p := NormalizePhone(raw)
	v.Check(phoneRe.MatchString(p), field, "Numéro invalide : format international attendu (+221...).")
	return &p
}

// Merge applique la saisie à une fiche (vide à la création) et valide le résultat.
func (in Input) Merge(base Fields) (Fields, error) {
	var v apperr.Validator
	out := base
	if in.Name != nil {
		out.Name = strings.TrimSpace(spacesRe.ReplaceAllString(*in.Name, " "))
	}
	v.Check(len(out.Name) >= 2 && len(out.Name) <= 160, "name", "Le nom doit contenir entre 2 et 160 caractères.")
	if in.Phone != nil {
		out.Phone = normalizePhoneField(&v, "phone", *in.Phone)
	}
	if in.Email != nil {
		out.Email = normalizeEmail(&v, "email", *in.Email)
	}
	if in.LegalName != nil {
		out.LegalName = optional(*in.LegalName)
		v.Check(out.LegalName == nil || (len(*out.LegalName) >= 2 && len(*out.LegalName) <= 200), "legal_name", "La raison sociale doit contenir entre 2 et 200 caractères.")
	}
	if in.LegalForm != nil {
		out.LegalForm = optional(*in.LegalForm)
		v.Check(out.LegalForm == nil || slices.Contains(LegalForms, *out.LegalForm), "legal_form", "Forme juridique inconnue.")
	}
	if in.NINEA != nil {
		out.NINEA = optional(strings.NewReplacer(" ", "", ".", "", "-", "", "/", "").Replace(strings.ToUpper(*in.NINEA)))
		v.Check(out.NINEA == nil || nineaRe.MatchString(*out.NINEA), "ninea", "NINEA invalide : 7 à 9 chiffres, suivis du code COFI (ex. 004567891 2G3).")
	}
	if in.RCCM != nil {
		out.RCCM = optional(strings.ToUpper(*in.RCCM))
		v.Check(out.RCCM == nil || rccmRe.MatchString(*out.RCCM), "rccm", "RCCM invalide (ex. SN-DKR-2015-B-12345).")
	}
	if in.Address != nil {
		out.Address = optional(*in.Address)
		v.Check(out.Address == nil || len(*out.Address) <= 300, "address", "Adresse trop longue (300 caractères au plus).")
	}
	if in.City != nil {
		out.City = optional(*in.City)
		v.Check(out.City == nil || len(*out.City) <= 100, "city", "Ville trop longue (100 caractères au plus).")
	}
	if in.Notes != nil {
		out.Notes = optional(*in.Notes)
		v.Check(out.Notes == nil || len(*out.Notes) <= 2000, "notes", "Notes trop longues (2 000 caractères au plus).")
	}
	if in.Activities != nil {
		out.Activities = []string{}
		for _, a := range *in.Activities {
			v.Check(slices.Contains(Activities, a), "activities", "Activité inconnue.")
			if !slices.Contains(out.Activities, a) {
				out.Activities = append(out.Activities, a)
			}
		}
	}
	if out.Activities == nil {
		out.Activities = []string{}
	}
	return out, v.Err()
}

// ContactInput décrit un contact chez le fournisseur.
type ContactInput struct {
	FullName  string  `json:"full_name"`
	JobTitle  *string `json:"job_title"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	IsPrimary bool    `json:"is_primary"`
	Notes     *string `json:"notes"`
}

// ContactFields est un contact normalisé.
type ContactFields struct {
	FullName                      string
	JobTitle, Phone, Email, Notes *string
	IsPrimary                     bool
}

// Normalize valide un contact : un nom, et au moins un moyen de le joindre.
func (in ContactInput) Normalize() (ContactFields, error) {
	var v apperr.Validator
	out := ContactFields{FullName: strings.TrimSpace(spacesRe.ReplaceAllString(in.FullName, " ")), IsPrimary: in.IsPrimary}
	v.Check(len(out.FullName) >= 2 && len(out.FullName) <= 160, "full_name", "Le nom doit contenir entre 2 et 160 caractères.")
	if in.JobTitle != nil {
		out.JobTitle = optional(*in.JobTitle)
		v.Check(out.JobTitle == nil || len(*out.JobTitle) <= 100, "job_title", "Fonction trop longue (100 caractères au plus).")
	}
	if in.Phone != nil {
		out.Phone = normalizePhoneField(&v, "phone", *in.Phone)
	}
	if in.Email != nil {
		out.Email = normalizeEmail(&v, "email", *in.Email)
	}
	if in.Notes != nil {
		out.Notes = optional(*in.Notes)
		v.Check(out.Notes == nil || len(*out.Notes) <= 500, "notes", "Notes trop longues (500 caractères au plus).")
	}
	v.Check(out.Phone != nil || out.Email != nil, "phone", "Indiquez un téléphone ou un email.")
	return out, v.Err()
}

// TermsInput décrit une nouvelle version des conditions commerciales.
type TermsInput struct {
	EffectiveFrom      *civil.Date      `json:"effective_from"`
	PaymentMethod      *string          `json:"payment_method"`
	PaymentDelayDays   *int16           `json:"payment_delay_days"`
	AdvancePct         *int16           `json:"advance_pct"`
	DiscountPct        *decimal.Decimal `json:"discount_pct"`
	DeliveryDays       *int16           `json:"delivery_days"`
	DeliveryIncluded   *bool            `json:"delivery_included"`
	MinimumOrderAmount *int64           `json:"minimum_order_amount"`
	CreditLimit        *int64           `json:"credit_limit"`
	Notes              *string          `json:"notes"`
}

// Validate applique les bornes des conditions commerciales.
func (in *TermsInput) Validate() error {
	var v apperr.Validator
	if in.Notes != nil {
		in.Notes = optional(*in.Notes)
	}
	if in.PaymentMethod != nil && *in.PaymentMethod == "" {
		in.PaymentMethod = nil
	}
	v.Check(in.PaymentMethod == nil || slices.Contains(PaymentMethods, *in.PaymentMethod), "payment_method", "Moyen de paiement inconnu.")
	v.Check(in.PaymentDelayDays == nil || (*in.PaymentDelayDays >= 0 && *in.PaymentDelayDays <= 365), "payment_delay_days", "Le délai de paiement doit être compris entre 0 et 365 jours.")
	v.Check(in.AdvancePct == nil || (*in.AdvancePct >= 0 && *in.AdvancePct <= 100), "advance_pct", "L'acompte doit être compris entre 0 et 100 %.")
	v.Check(in.DiscountPct == nil || (!in.DiscountPct.IsNegative() && in.DiscountPct.LessThanOrEqual(decimal.NewFromInt(100)) && in.DiscountPct.Equal(in.DiscountPct.Truncate(2))),
		"discount_pct", "La remise doit être comprise entre 0 et 100 %, avec 2 décimales au plus.")
	v.Check(in.DeliveryDays == nil || (*in.DeliveryDays >= 0 && *in.DeliveryDays <= 365), "delivery_days", "Le délai de livraison doit être compris entre 0 et 365 jours.")
	v.Check(in.MinimumOrderAmount == nil || *in.MinimumOrderAmount >= 0, "minimum_order_amount", "Le minimum de commande ne peut pas être négatif.")
	v.Check(in.CreditLimit == nil || *in.CreditLimit >= 0, "credit_limit", "L'encours autorisé ne peut pas être négatif.")
	v.Check(in.Notes == nil || len(*in.Notes) <= 2000, "notes", "Notes trop longues (2 000 caractères au plus).")
	empty := in.PaymentMethod == nil && in.PaymentDelayDays == nil && in.AdvancePct == nil && in.DiscountPct == nil &&
		in.DeliveryDays == nil && in.DeliveryIncluded == nil && in.MinimumOrderAmount == nil && in.CreditLimit == nil && in.Notes == nil
	v.Check(!empty, "payment_method", "Renseignez au moins une condition.")
	return v.Err()
}
