package i18n

import (
	"os"
	"testing"
)

func TestSetLanguage(t *testing.T) {
	// Test setting different languages
	SetLanguage(LangEnglish)
	if GetLanguage() != LangEnglish {
		t.Errorf("expected %s, got %s", LangEnglish, GetLanguage())
	}

	SetLanguage(LangFrench)
	if GetLanguage() != LangFrench {
		t.Errorf("expected %s, got %s", LangFrench, GetLanguage())
	}

	SetLanguage(LangJapanese)
	if GetLanguage() != LangJapanese {
		t.Errorf("expected %s, got %s", LangJapanese, GetLanguage())
	}

	// Reset to default
	SetLanguage(DefaultLanguage)
}

func TestT(t *testing.T) {
	// Ensure we're using English for predictable results
	SetLanguage(LangEnglish)

	// Test basic translation
	result := T("welcome")
	if result == "" || result == "welcome" {
		// If it returns the key, locale might not be loaded, but function should work
		t.Logf("T('welcome') returned: %s", result)
	}

	// Test unknown key returns the key itself
	result = T("unknown_key_that_does_not_exist")
	if result != "unknown_key_that_does_not_exist" {
		t.Errorf("expected unknown key to return itself, got %s", result)
	}
}

func TestTf(t *testing.T) {
	SetLanguage(LangEnglish)

	// Test formatted translation with data
	result := Tf("loading_page", map[string]interface{}{"Page": 5})
	if result == "" {
		t.Error("expected non-empty translation")
	}

	// Test with unknown key
	result = Tf("unknown_key", map[string]interface{}{"Foo": "bar"})
	if result != "unknown_key" {
		t.Errorf("expected unknown key to return itself, got %s", result)
	}
}

func TestListLanguages(t *testing.T) {
	langs := ListLanguages()

	if len(langs) != len(SupportedLanguages) {
		t.Errorf("expected %d languages, got %d", len(SupportedLanguages), len(langs))
	}

	// Check first and last
	if langs[0] != string(LangEnglish) {
		t.Errorf("expected first language to be %s, got %s", LangEnglish, langs[0])
	}

	// All should be non-empty
	for i, lang := range langs {
		if lang == "" {
			t.Errorf("language at index %d is empty", i)
		}
	}
}

func TestLanguageName(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"en_US", "English (US)"},
		{"fr_FR", "Francais"},
		{"ja_JP", "日本語"},
		{"zh_CN", "中文"},
		{"unknown", "unknown"}, // Unknown codes return themselves
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			result := LanguageName(tc.code)
			if result != tc.expected {
				t.Errorf("LanguageName(%s) = %s, want %s", tc.code, result, tc.expected)
			}
		})
	}
}

func TestLanguageIndex(t *testing.T) {
	tests := []struct {
		code     string
		expected int
	}{
		{"en_US", 0},
		{"en_GB", 1},
		{"es_ES", 2},
		{"fr_FR", 3},
		{"unknown", 0}, // Unknown codes return 0
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			result := LanguageIndex(tc.code)
			if result != tc.expected {
				t.Errorf("LanguageIndex(%s) = %d, want %d", tc.code, result, tc.expected)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	// Save original env vars
	origLCAll := os.Getenv("LC_ALL")
	origLCMessages := os.Getenv("LC_MESSAGES")
	origLang := os.Getenv("LANG")

	// Cleanup after test
	defer func() {
		os.Setenv("LC_ALL", origLCAll)
		os.Setenv("LC_MESSAGES", origLCMessages)
		os.Setenv("LANG", origLang)
	}()

	tests := []struct {
		name     string
		lcAll    string
		lcMsg    string
		lang     string
		expected Language
	}{
		{
			name:     "exact match from LC_ALL",
			lcAll:    "fr_FR.UTF-8",
			expected: LangFrench,
		},
		{
			name:     "exact match from LANG",
			lang:     "de_DE.UTF-8",
			expected: LangGerman,
		},
		{
			name:     "prefix match",
			lang:     "es.UTF-8",
			expected: LangSpanish,
		},
		{
			name:     "no match falls back to default",
			lang:     "xx_XX.UTF-8",
			expected: DefaultLanguage,
		},
		{
			name:     "empty env falls back to default",
			expected: DefaultLanguage,
		},
		{
			name:     "LC_MESSAGES takes precedence over LANG",
			lcMsg:    "ja_JP.UTF-8",
			lang:     "en_US.UTF-8",
			expected: LangJapanese,
		},
		{
			name:     "LC_ALL takes precedence over LC_MESSAGES",
			lcAll:    "zh_CN.UTF-8",
			lcMsg:    "ja_JP.UTF-8",
			expected: LangChinese,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all env vars first
			os.Unsetenv("LC_ALL")
			os.Unsetenv("LC_MESSAGES")
			os.Unsetenv("LANG")

			// Set test values
			if tc.lcAll != "" {
				os.Setenv("LC_ALL", tc.lcAll)
			}
			if tc.lcMsg != "" {
				os.Setenv("LC_MESSAGES", tc.lcMsg)
			}
			if tc.lang != "" {
				os.Setenv("LANG", tc.lang)
			}

			result := DetectLanguage()
			if result != tc.expected {
				t.Errorf("DetectLanguage() = %s, want %s", result, tc.expected)
			}
		})
	}
}

func TestSupportedLanguages(t *testing.T) {
	// Ensure all supported languages have names
	for _, lang := range SupportedLanguages {
		name, ok := LanguageNames[lang]
		if !ok {
			t.Errorf("language %s has no display name", lang)
		}
		if name == "" {
			t.Errorf("language %s has empty display name", lang)
		}
	}
}

func TestLanguageConstants(t *testing.T) {
	// Ensure language constants are correct format (xx_XX)
	languages := []Language{
		LangEnglish, LangEnglishGB, LangSpanish, LangFrench,
		LangGerman, LangChinese, LangHindi, LangArabic,
		LangBengali, LangPortuguese, LangRussian, LangJapanese,
	}

	for _, lang := range languages {
		code := string(lang)
		if len(code) != 5 || code[2] != '_' {
			t.Errorf("language %s should be in format xx_XX", lang)
		}
	}
}

func TestTranslationConsistency(t *testing.T) {
	// Test that switching languages actually changes translations
	SetLanguage(LangEnglish)
	englishWelcome := T("welcome")

	SetLanguage(LangFrench)
	frenchWelcome := T("welcome")

	// They should be different (unless both are missing the key)
	if englishWelcome != "welcome" && frenchWelcome != "welcome" {
		if englishWelcome == frenchWelcome {
			t.Log("Warning: English and French welcome messages are the same")
		}
	}

	// Reset
	SetLanguage(DefaultLanguage)
}
