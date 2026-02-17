package i18n

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// Language represents a supported language
type Language string

const (
	// LangEnglish is US English
	LangEnglish Language = "en_US"
	// LangEnglishGB is British English
	LangEnglishGB Language = "en_GB"
	// LangSpanish is Spanish
	LangSpanish Language = "es_ES"
	// LangFrench is French
	LangFrench Language = "fr_FR"
	// LangGerman is German
	LangGerman Language = "de_DE"
	// LangChinese is Simplified Chinese
	LangChinese Language = "zh_CN"
	// LangHindi is Hindi
	LangHindi Language = "hi_IN"
	// LangArabic is Arabic
	LangArabic Language = "ar_SA"
	// LangBengali is Bengali
	LangBengali Language = "bn_BD"
	// LangPortuguese is Brazilian Portuguese
	LangPortuguese Language = "pt_BR"
	// LangRussian is Russian
	LangRussian Language = "ru_RU"
	// LangJapanese is Japanese
	LangJapanese Language = "ja_JP"
)

// DefaultLanguage is the fallback language
const DefaultLanguage = LangEnglish

// SupportedLanguages returns all available languages
var SupportedLanguages = []Language{
	LangEnglish,
	LangEnglishGB,
	LangSpanish,
	LangFrench,
	LangGerman,
	LangChinese,
	LangHindi,
	LangArabic,
	LangBengali,
	LangPortuguese,
	LangRussian,
	LangJapanese,
}

// LanguageNames maps language codes to display names
var LanguageNames = map[Language]string{
	LangEnglish:    "English (US)",
	LangEnglishGB:  "English (UK)",
	LangSpanish:    "Espanol",
	LangFrench:     "Francais",
	LangGerman:     "Deutsch",
	LangChinese:    "中文",
	LangHindi:      "हिन्दी",
	LangArabic:     "العربية",
	LangBengali:    "বাংলা",
	LangPortuguese: "Português",
	LangRussian:    "Русский",
	LangJapanese:   "日本語",
}

var bundle *i18n.Bundle
var localizer *i18n.Localizer
var currentLang = DefaultLanguage

// flattenNestedYAML flattens nested YAML maps into dot-notation keys
// e.g., {common: {error: "Error"}} -> {"common.error": {other: "Error"}}
func flattenNestedYAML(data []byte, v interface{}) error {
	// First, unmarshal into a generic map
	var nested map[string]interface{}
	if err := yaml.Unmarshal(data, &nested); err != nil {
		return err
	}

	// Flatten the nested map
	flat := make(map[string]interface{})
	flattenMap("", nested, flat)

	// Convert back to YAML and unmarshal into the target
	flatYAML, err := yaml.Marshal(flat)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(flatYAML, v)
}

// flattenMap recursively flattens a nested map using dot notation
func flattenMap(prefix string, nested, flat map[string]interface{}) {
	for key, value := range nested {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// Check if this is a go-i18n message format (has "other" key with string value)
			if other, ok := v["other"]; ok {
				if _, isString := other.(string); isString {
					// This is a leaf message node
					flat[fullKey] = v
					continue
				}
			}
			// Otherwise, recurse into nested map
			flattenMap(fullKey, v, flat)
		default:
			// For simple values, wrap in go-i18n format
			flat[fullKey] = map[string]interface{}{"other": fmt.Sprintf("%v", v)}
		}
	}
}

func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", flattenNestedYAML)

	// Load embedded locale files
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/en_US.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/en_GB.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/es_ES.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/fr_FR.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/de_DE.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/zh_CN.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/hi_IN.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/ar_SA.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/bn_BD.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/pt_BR.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/ru_RU.yaml")
	_, _ = bundle.LoadMessageFileFS(localeFS, "locales/ja_JP.yaml")

	// Default to English
	localizer = i18n.NewLocalizer(bundle, string(LangEnglish))
}

// SetLanguage sets the current language
func SetLanguage(lang Language) {
	currentLang = lang
	localizer = i18n.NewLocalizer(bundle, string(lang))
}

// GetLanguage returns the current language
func GetLanguage() Language {
	return currentLang
}

// T returns the translation for the given message ID
func T(msgID string) string {
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID: msgID,
	})
	if err != nil {
		return msgID
	}
	return msg
}

// Tf returns a formatted translation with template data
func Tf(msgID string, data map[string]interface{}) string {
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    msgID,
		TemplateData: data,
	})
	if err != nil {
		return msgID
	}
	return msg
}

// ListLanguages returns language codes for selector
func ListLanguages() []string {
	result := make([]string, len(SupportedLanguages))
	for i, lang := range SupportedLanguages {
		result[i] = string(lang)
	}
	return result
}

// LanguageName returns the display name for a language code
func LanguageName(code string) string {
	if name, ok := LanguageNames[Language(code)]; ok {
		return name
	}
	return code
}

// LanguageIndex returns the index of the given language code
func LanguageIndex(code string) int {
	for i, lang := range SupportedLanguages {
		if string(lang) == code {
			return i
		}
	}
	return 0 // Default to first language
}

// DetectLanguage detects the system locale and returns a supported language.
// Falls back to DefaultLanguage (en_US) if not supported.
func DetectLanguage() Language {
	// Check LANG and LC_ALL environment variables
	for _, envVar := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale := os.Getenv(envVar); locale != "" {
			// Locale format: en_US.UTF-8 or fr_FR.UTF-8
			// Strip encoding suffix
			locale = strings.Split(locale, ".")[0]

			// Check if exact match
			for _, lang := range SupportedLanguages {
				if string(lang) == locale {
					return lang
				}
			}

			// Check language prefix (e.g., "fr" matches "fr_FR")
			langPrefix := strings.Split(locale, "_")[0]
			for _, lang := range SupportedLanguages {
				if strings.HasPrefix(string(lang), langPrefix+"_") {
					return lang
				}
			}
		}
	}

	return DefaultLanguage
}
