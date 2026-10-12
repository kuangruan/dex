package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestResolveLocale(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		headers map[string]string
		want    Locale
	}{
		{name: "defaults to english", url: "/auth", want: LocaleEnglish},
		{name: "accept-language chinese", url: "/auth", headers: map[string]string{"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8"}, want: LocaleChinese},
		{name: "accept-language english", url: "/auth", headers: map[string]string{"Accept-Language": "en-US,en;q=0.9"}, want: LocaleEnglish},
		{name: "accept-language quality order", url: "/auth", headers: map[string]string{"Accept-Language": "en;q=0.3,zh;q=0.9"}, want: LocaleChinese},
		{name: "query overrides header", url: "/auth?lang=zh", headers: map[string]string{"Accept-Language": "en-US,en;q=0.9"}, want: LocaleChinese},
		{name: "unknown tag ignored", url: "/auth?lang=de-DE", headers: map[string]string{"Accept-Language": "zh-CN"}, want: LocaleChinese},
		{name: "garbage header", url: "/auth", headers: map[string]string{"Accept-Language": ";;;q=abc"}, want: LocaleEnglish},
	}

	factory := newI18nFactory(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.url, nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := resolveLocale(r, factory.dict); got != tt.want {
				t.Fatalf("resolveLocale = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveLocaleCookie(t *testing.T) {
	factory := newI18nFactory(nil)
	r := httptest.NewRequest(http.MethodGet, "/auth", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.AddCookie(&http.Cookie{Name: langCookieName, Value: "zh"})
	if got := resolveLocale(r, factory.dict); got != LocaleChinese {
		t.Fatalf("cookie locale = %q, want %q", got, LocaleChinese)
	}
}

func TestTranslator(t *testing.T) {
	factory := newI18nFactory(map[string]map[string]string{
		// override a bundled entry and add one for the same locale
		"zh": {"logInTitle": "登录", "customKey": "自定义"},
		// add a locale that has no bundled dictionary at all
		"de": {"logInTitle": "Anmelden"},
	})

	zh := &Translator{dict: factory.dict, locale: LocaleChinese}
	if got := zh.T("logInTitle"); got != "登录" {
		t.Errorf("override en zh: got %q", got)
	}
	if got := zh.T("customKey"); got != "自定义" {
		t.Errorf("custom key: got %q", got)
	}
	if got := zh.T("username"); got != "用户名" {
		t.Errorf("bundled zh entry: got %q", got)
	}
	// a key missing in the requested locale falls back to English
	de := &Translator{dict: factory.dict, locale: "de"}
	if got := de.T("username"); got != "Username" {
		t.Errorf("english fallback: got %q", got)
	}
	// missing everywhere falls back to the key, or to TOr's fallback
	if got := zh.T("nope.nope"); got != "nope.nope" {
		t.Errorf("key fallback: got %q", got)
	}
	if got := zh.TOr("nope.nope", "Use a nicer message", 1); got != "Use a nicer message" {
		t.Errorf("TOr fallback: got %q", got)
	}
	// the extra locale is selectable and translated
	en := httptest.NewRequest(http.MethodGet, "/auth", nil)
	en.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	if got := factory.translator(en).T("logInTitle"); got != "Anmelden" {
		t.Errorf("added locale: got %q", got)
	}
	// arguments are formatted
	if got := zh.T("invalidCredentials", "Email"); got != "Email或密码错误。" {
		t.Errorf("formatted message: got %q", got)
	}
}

// TestRenderLocalizedPages renders the shipped templates and asserts the language
// actually reaches the page.
func TestRenderLocalizedPages(t *testing.T) {
	tmpls, err := loadTemplates(webConfig{webFS: os.DirFS("../web"), issuer: "NetBird"}, "templates")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	connectors := []connectorInfo{{ID: "local", Name: "Email", Type: "local", URL: "/auth/local"}}
	render := func(t *testing.T, lang string, run func(r *http.Request, w http.ResponseWriter) error) string {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/auth", nil)
		if lang != "" {
			r.Header.Set("Accept-Language", lang)
		}
		w := httptest.NewRecorder()
		if err := run(r, w); err != nil {
			t.Fatalf("render: %v", err)
		}
		return w.Body.String()
	}

	t.Run("login english by default", func(t *testing.T) {
		body := render(t, "", func(r *http.Request, w http.ResponseWriter) error {
			return tmpls.login(r, w, connectors)
		})
		if !strings.Contains(body, "Log in with Email") {
			t.Errorf("expected english connector label, got:\n%s", body)
		}
	})
	t.Run("login chinese", func(t *testing.T) {
		body := render(t, "zh-CN,zh;q=0.9", func(r *http.Request, w http.ResponseWriter) error {
			return tmpls.login(r, w, connectors)
		})
		if !strings.Contains(body, "使用Email登录") {
			t.Errorf("expected chinese connector label, got:\n%s", body)
		}
		if !strings.Contains(body, `lang="zh"`) {
			t.Errorf("expected html lang=zh, got:\n%s", body)
		}
	})
	t.Run("password chinese", func(t *testing.T) {
		body := render(t, "zh-CN,zh;q=0.9", func(r *http.Request, w http.ResponseWriter) error {
			return tmpls.password(r, w, "/auth/local", "user@example.com", "Email", true, "/auth", nil)
		})
		for _, want := range []string{"登录你的账号", "密码", "Email或密码错误。"} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in the password page, got:\n%s", want, body)
			}
		}
	})
	t.Run("password english", func(t *testing.T) {
		body := render(t, "en-US,en;q=0.9", func(r *http.Request, w http.ResponseWriter) error {
			return tmpls.password(r, w, "/auth/local", "user@example.com", "Email", true, "/auth", nil)
		})
		for _, want := range []string{"Log in to Your Account", "Invalid Email and password."} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in the password page, got:\n%s", want, body)
			}
		}
	})
	t.Run("approval scopes translated", func(t *testing.T) {
		body := render(t, "zh-CN,zh;q=0.9", func(r *http.Request, w http.ResponseWriter) error {
			return tmpls.approval(r, w, "req-1", "user@example.com", "netbird-dashboard", []string{"openid", "email", "offline_access"})
		})
		for _, want := range []string{"授予访问权限", "查看你的邮箱地址", "允许离线访问"} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in the approval page, got:\n%s", want, body)
			}
		}
	})
}

func TestTemplateFallbackHelper(t *testing.T) {
	factory := newI18nFactory(map[string]map[string]string{"zh": {"prompt.Email": "邮箱"}})
	data := factory.data(httptest.NewRequest(http.MethodGet, "/auth?lang=zh", nil))
	if got := data.TOr("prompt.Email", "Email"); got != "邮箱" {
		t.Errorf("TOr zh entry: got %q", got)
	}
	if got := data.TOr("prompt.Username", "Username"); got != "Username" {
		t.Errorf("TOr fallback: got %q", got)
	}
	if got := data.TOr("prompt.Username", "Username", 1); got != "Username" {
		t.Errorf("TOr fallback ignores args: got %q", got)
	}
}
