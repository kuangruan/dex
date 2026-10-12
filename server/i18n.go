package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Locale identifies a UI language.
type Locale string

const (
	LocaleEnglish Locale = "en"
	LocaleChinese Locale = "zh"
)

const (
	// langQueryParam pins the language for a single request, e.g. /auth?lang=zh.
	langQueryParam = "lang"
	// langCookieName pins the language for subsequent requests.
	langCookieName = "dex_lang"
)

// SupportedLocales lists the locales with a bundled dictionary, in fallback order.
// A locale added through WebConfig.Translations is selectable as well.
var SupportedLocales = []Locale{LocaleEnglish, LocaleChinese}

// defaultTranslations holds the dictionaries for the templates shipped in web/templates.
// Embedders add or override entries through WebConfig.Translations; unknown keys fall
// back to English and finally to the key itself, so a missing entry never renders empty.
var defaultTranslations = map[Locale]map[string]string{
	LocaleEnglish: {
		"logInTo":                 "Log in to %s",
		"logInWith":               "Log in with %s",
		"logInTitle":              "Log in to Your Account",
		"password":                "Password",
		"passwordPlaceholder":     "password",
		"login":                   "Login",
		"selectAnotherMethod":     "Select another login method.",
		"invalidCredentials":      "Invalid %s and password.",
		"rememberMe":              "Remember me",
		"enterUserCode":           "Enter User Code",
		"submit":                  "Submit",
		"loginSuccessfulFor":      "Login Successful for %s",
		"returnToDevice":          "Return to your device to continue",
		"grantAccess":             "Grant Access",
		"cancel":                  "Cancel",
		"twoFactor":               "Two-factor authentication",
		"scanQr":                  "Scan the QR code below using your authenticator app, then enter the code.",
		"enterCodeFromApp":        "Enter the code from your authenticator app.",
		"oneTimeCode":             "One-time code",
		"verify":                  "Verify",
		"registerSecurityKey":     "Register security key",
		"registerSecurityKeyDesc": "Register a security key for two-factor authentication.",
		"useSecurityKey":          "Use your security key to verify your identity.",
		"waitingForSecurityKey":   "Waiting for security key...",
		"loggedOut":               "Logged Out",
		"loggedOutDesc":           "You have been successfully logged out.",
		"sessionNotFound":         "Session Not Found",
		"sessionNotFoundDesc":     "No active session found.",
		"backToApplication":       "← Back to Application",
		"loginSuccessful":         "Login Successful",
		"copyCodeBack":            "Please copy this code, switch to your application and paste it there:",
		"username":                "Username",
		"email":                   "Email",
		"groups":                  "Groups",
		"groupsCountOne":          "%d group",
		"groupsCountMany":         "%d groups",
		"connector":               "Connector",
		"ipAddress":               "IP address",
		"browser":                 "Browser",
		"lastLogin":               "Last login",
		"logout":                  "Logout",
		"notLoggedIn":             "Not logged in",
		"discovery":               "Discovery",
		"scope.offline_access":    "Have offline access",
		"scope.profile":           "View basic profile information",
		"scope.email":             "View your email address",
		"scope.groups":            "View your groups",
	},
	LocaleChinese: {
		"logInTo":                 "登录 %s",
		"logInWith":               "使用%s登录",
		"logInTitle":              "登录你的账号",
		"password":                "密码",
		"passwordPlaceholder":     "密码",
		"login":                   "登录",
		"selectAnotherMethod":     "选择其他登录方式。",
		"invalidCredentials":      "%s或密码错误。",
		"rememberMe":              "记住我",
		"enterUserCode":           "输入用户代码",
		"submit":                  "提交",
		"loginSuccessfulFor":      "%s 登录成功",
		"returnToDevice":          "回到你的设备继续操作",
		"grantAccess":             "授予访问权限",
		"cancel":                  "取消",
		"twoFactor":               "双重认证",
		"scanQr":                  "用验证器应用扫描下方二维码，然后输入验证码。",
		"enterCodeFromApp":        "输入验证器应用中的验证码。",
		"oneTimeCode":             "一次性验证码",
		"verify":                  "验证",
		"registerSecurityKey":     "注册安全密钥",
		"registerSecurityKeyDesc": "注册用于双重认证的安全密钥。",
		"useSecurityKey":          "使用你的安全密钥验证身份。",
		"waitingForSecurityKey":   "正在等待安全密钥…",
		"loggedOut":               "已退出登录",
		"loggedOutDesc":           "你已成功退出登录。",
		"sessionNotFound":         "未找到会话",
		"sessionNotFoundDesc":     "没有活跃的会话。",
		"backToApplication":       "← 返回应用",
		"loginSuccessful":         "登录成功",
		"copyCodeBack":            "请复制此代码，切换到你的应用并粘贴：",
		"username":                "用户名",
		"email":                   "邮箱",
		"groups":                  "用户组",
		"groupsCountOne":          "%d 个用户组",
		"groupsCountMany":         "%d 个用户组",
		"connector":               "连接器",
		"ipAddress":               "IP 地址",
		"browser":                 "浏览器",
		"lastLogin":               "上次登录",
		"logout":                  "退出登录",
		"notLoggedIn":             "未登录",
		"discovery":               "发现文档",
		"scope.offline_access":    "允许离线访问",
		"scope.profile":           "查看基本资料",
		"scope.email":             "查看你的邮箱地址",
		"scope.groups":            "查看你的用户组",
	},
}

// Translator resolves message keys for one locale.
type Translator struct {
	dict   map[Locale]map[string]string
	locale Locale
}

// Locale returns the resolved locale.
func (t *Translator) Locale() Locale { return t.locale }

// T returns the message for key, formatted with args when given. Missing keys fall
// back to English and finally to the key itself.
func (t *Translator) T(key string, args ...interface{}) string {
	return t.TOr(key, key, args...)
}

// TOr behaves like T but uses fallback when the key is unknown in every dictionary.
func (t *Translator) TOr(key, fallback string, args ...interface{}) string {
	msg, ok := t.dict[t.locale][key]
	if !ok {
		if msg, ok = t.dict[LocaleEnglish][key]; !ok {
			// The fallback is a plain string, never a format string.
			return fallback
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// i18nData is embedded in every template data struct so templates can call
// {{ .T "key" }} / {{ .T "key" arg }} and read {{ .Locale }}.
type i18nData struct {
	Locale string
	tr     *Translator
}

// T translates key for the request locale; templates call it as {{ .T "key" }}.
func (d i18nData) T(key string, args ...interface{}) string {
	return d.tr.T(key, args...)
}

// TOr behaves like T but renders fallback when key is unknown, for templates that
// translate values coming from configuration ({{ .TOr (printf "prompt.%s" .Prompt) .Prompt }}).
func (d i18nData) TOr(key, fallback string, args ...interface{}) string {
	return d.tr.TOr(key, fallback, args...)
}

// i18nFactory builds the per-request translation data from the bundled dictionaries
// merged with the operator-supplied WebConfig.Translations.
type i18nFactory struct {
	dict map[Locale]map[string]string
}

func newI18nFactory(custom map[string]map[string]string) *i18nFactory {
	dict := make(map[Locale]map[string]string, len(defaultTranslations)+len(custom))
	for locale, entries := range defaultTranslations {
		m := make(map[string]string, len(entries))
		for k, v := range entries {
			m[k] = v
		}
		dict[locale] = m
	}
	for tag, entries := range custom {
		locale := normalizeLocale(tag)
		if locale == "" {
			continue
		}
		m := dict[locale]
		if m == nil {
			m = map[string]string{}
			dict[locale] = m
		}
		for k, v := range entries {
			m[k] = v
		}
	}
	return &i18nFactory{dict: dict}
}

// translator resolves the request locale and returns a translator for it.
func (f *i18nFactory) translator(r *http.Request) *Translator {
	return &Translator{dict: f.dict, locale: resolveLocale(r, f.dict)}
}

// data returns the values handed to the templates.
func (f *i18nFactory) data(r *http.Request) i18nData {
	tr := f.translator(r)
	return i18nData{Locale: string(tr.locale), tr: tr}
}

// resolveLocale picks the UI language: explicit ?lang= wins, then the dex_lang
// cookie, then the best match from Accept-Language, finally English.
func resolveLocale(r *http.Request, dict map[Locale]map[string]string) Locale {
	if loc := matchLocale(r.URL.Query().Get(langQueryParam), dict); loc != "" {
		return loc
	}
	if c, err := r.Cookie(langCookieName); err == nil {
		if loc := matchLocale(c.Value, dict); loc != "" {
			return loc
		}
	}
	for _, tag := range acceptedLanguages(r.Header.Get("Accept-Language")) {
		if loc := matchLocale(tag, dict); loc != "" {
			return loc
		}
	}
	return LocaleEnglish
}

// normalizeLocale lowercases and trims a language tag, returning "" when it is empty.
func normalizeLocale(tag string) Locale {
	tag = strings.ToLower(strings.TrimSpace(tag))
	return Locale(tag)
}

// matchLocale returns the locale for tag when a dictionary exists for it, its base
// subtag (zh-CN -> zh) or nothing otherwise.
func matchLocale(tag string, dict map[Locale]map[string]string) Locale {
	locale := normalizeLocale(tag)
	if locale == "" {
		return ""
	}
	if dict[locale] != nil {
		return locale
	}
	if i := strings.IndexAny(string(locale), "-_"); i > 0 {
		base := Locale(locale[:i])
		if dict[base] != nil {
			return base
		}
	}
	return ""
}

type acceptedLanguage struct {
	tag string
	q   float64
}

// acceptedLanguages parses an Accept-Language header into tags ordered by q-value.
func acceptedLanguages(header string) []string {
	var accepted []acceptedLanguage
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag, quality := part, 1.0
		if i := strings.Index(part, ";"); i >= 0 {
			tag = strings.TrimSpace(part[:i])
			for _, param := range strings.Split(part[i+1:], ";") {
				param = strings.TrimSpace(param)
				if strings.HasPrefix(param, "q=") {
					if v, err := strconv.ParseFloat(strings.TrimSpace(param[2:]), 64); err == nil {
						quality = v
					}
				}
			}
		}
		if quality > 0 {
			accepted = append(accepted, acceptedLanguage{tag: tag, q: quality})
		}
	}
	sort.SliceStable(accepted, func(i, j int) bool { return accepted[i].q > accepted[j].q })
	tags := make([]string, 0, len(accepted))
	for _, a := range accepted {
		tags = append(tags, a.tag)
	}
	return tags
}
