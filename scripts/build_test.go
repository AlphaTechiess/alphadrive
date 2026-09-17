package main

import (
	"os"
	"strings"
	"testing"
)

func TestMinifyCSS(t *testing.T) {
	// Test 1: Required prompt example
	input1 := `.auth-page {
    background: var(--bg-main);
    min-height: 100vh;
}`
	out1 := MinifyCSS(input1)
	if !strings.Contains(out1, ".auth-page{") {
		t.Fatalf("expected out1 to contain '.auth-page{', got %q", out1)
	}
	if !strings.Contains(out1, "background:var(--bg-main)") {
		t.Fatalf("expected out1 to contain 'background:var(--bg-main)', got %q", out1)
	}
	if !strings.HasSuffix(out1, "}") {
		t.Fatalf("expected out1 to end with '}', got %q", out1)
	}

	// Test 2: Combinators (.a > .b, .a + .b, .a, .b)
	input2 := `
.a > .b {
    color: red;
}
.a + .b {
    color: blue;
}
.a, .b {
    color: green;
}
`
	out2 := MinifyCSS(input2)
	if !strings.Contains(out2, ".a > .b{") && !strings.Contains(out2, ".a>.b{") {
		t.Fatalf("expected out2 to contain child combinator, got %q", out2)
	}
	if !strings.Contains(out2, ".a + .b{") && !strings.Contains(out2, ".a+.b{") {
		t.Fatalf("expected out2 to contain adjacent combinator, got %q", out2)
	}
	if !strings.Contains(out2, ".a,.b{") {
		t.Fatalf("expected out2 to contain comma selector list, got %q", out2)
	}

	// Test 3: Properties, calc(), url(), strings, CSS custom properties
	input3 := `
:root {
    --bg-main: #081524;
    --font-stack: "Segoe UI", Roboto, sans-serif;
}
.box {
    margin: 0;
    width: calc(100% - 20px);
    background-image: url("data:image/svg+xml;utf8,<svg></svg>");
    content: "hello ; world { test }";
}
`
	out3 := MinifyCSS(input3)
	if !strings.Contains(out3, "--bg-main:#081524") {
		t.Fatalf("expected custom property, got %q", out3)
	}
	if !strings.Contains(out3, "margin:0") {
		t.Fatalf("expected margin:0, got %q", out3)
	}
	if !strings.Contains(out3, "calc(100% - 20px)") {
		t.Fatalf("expected calc(100%% - 20px), got %q", out3)
	}
	if !strings.Contains(out3, `url("data:image/svg+xml;utf8,<svg></svg>")`) {
		t.Fatalf("expected url literal, got %q", out3)
	}
	if !strings.Contains(out3, `content:"hello ; world { test }"`) {
		t.Fatalf("expected string literal with braces, got %q", out3)
	}

	// Test 4: @media and @keyframes
	input4 := `
@media (min-width: 768px) and (max-width: 1024px) {
    .container {
        padding: 24px;
    }
}
@keyframes spin {
    0% {
        transform: rotate(0deg);
    }
    100% {
        transform: rotate(360deg);
    }
}
`
	out4 := MinifyCSS(input4)
	if !strings.Contains(out4, "@media (min-width:768px) and (max-width:1024px)") && !strings.Contains(out4, "@media (min-width: 768px) and (max-width: 1024px)") {
		t.Fatalf("expected valid media query, got %q", out4)
	}
	if !strings.Contains(out4, "@keyframes spin") {
		t.Fatalf("expected keyframes rule, got %q", out4)
	}
	if !strings.Contains(out4, "transform:rotate(0deg)") || !strings.Contains(out4, "transform:rotate(360deg)") {
		t.Fatalf("expected keyframe steps, got %q", out4)
	}
}

func TestMinifyRealAppCSS(t *testing.T) {
	cssData, err := os.ReadFile("../web/static/css/app.css")
	if err != nil {
		cssData, err = os.ReadFile("web/static/css/app.css")
	}
	if err != nil {
		t.Skipf("cannot read app.css: %v", err)
	}

	minified := MinifyCSS(string(cssData))

	// Verify brace balance
	openBraces := strings.Count(minified, "{")
	closeBraces := strings.Count(minified, "}")
	if openBraces == 0 || openBraces != closeBraces {
		t.Fatalf("mismatched braces in minified CSS: %d '{' vs %d '}'", openBraces, closeBraces)
	}

	// Verify critical selectors from setup & auth pages
	criticalSelectors := []string{
		".auth-page",
		".auth-shell",
		".auth-logo",
		".auth-title",
		".auth-card",
		".form-group",
		".btn-submit",
	}

	for _, sel := range criticalSelectors {
		if !strings.Contains(minified, sel) {
			t.Fatalf("missing critical selector %q in minified CSS", sel)
		}
	}
}

func TestMinifyJS(t *testing.T) {
	input := `
// This is a line comment
function test() {
    /* block comment */
    let str = "https://example.com//test/*not_comment*/";
    let template = ` + "`" + `multiline
    // inside template literal
    ` + "`" + `;
    return str;
}
`
	out := MinifyJS(input)
	if strings.Contains(out, "This is a line comment") {
		t.Fatalf("expected line comment to be stripped, got %q", out)
	}
	if strings.Contains(out, "block comment") {
		t.Fatalf("expected block comment to be stripped, got %q", out)
	}
	if !strings.Contains(out, "https://example.com//test/*not_comment*/") {
		t.Fatalf("expected string literal with slashes to be preserved, got %q", out)
	}
	if !strings.Contains(out, "// inside template literal") {
		t.Fatalf("expected template literal content to be preserved, got %q", out)
	}
}

func TestMinifyHTML(t *testing.T) {
	input := `
<!DOCTYPE html>
<!-- Comment to strip -->
<html>
    <body>
        <span>Hello</span> <span>World</span>
    </body>
</html>
`
	out := MinifyHTML(input)
	if strings.Contains(out, "Comment to strip") {
		t.Fatalf("expected comment to be stripped, got %q", out)
	}
	if !strings.Contains(out, "<span>Hello</span> <span>World</span>") {
		t.Fatalf("expected inline spacing to be preserved, got %q", out)
	}
}
