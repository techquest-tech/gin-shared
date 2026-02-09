# Email Notifier Usage

## Inline Images in Templates

To embed images directly into the email body (inline images):

1.  **Pass the image file path** when calling the `Send` method.
2.  **Reference the image** in your HTML template using `cid:<filename>`.

### Example

Suppose you have an image named `logo.png` that you want to include in the email.

#### 1. Go Code

Pass the absolute path to `logo.png` as an attachment argument to `Send`.

```go
// ... initialize notifier ...

// Send email with "welcome" template and attach logo.png
err := notifier.Send("welcome", data, "/path/to/assets/logo.png")
if err != nil {
    // handle error
}
```

The `Send` method logic (in `smtp.go`) automatically detects that `logo.png` is an image and sets its `Content-ID` to the filename `logo.png`.

#### 2. HTML Template Configuration

In your `EmailTmpl` configuration, use `cid:logo.png` in the `src` attribute of the `<img>` tag.

```go
notifier.Template = map[string]*notify.EmailTmpl{
    "welcome": {
        Subject: "Welcome to our Service",
        // Use 'cid:' followed by the filename (base name) of the attached image
        Body: `
            <html>
                <body>
                    <h1>Hello, {{.Name}}</h1>
                    <p>Here is our logo:</p>
                    <img src="cid:logo.png" alt="Company Logo" />
                </body>
            </html>
        `,
        Receivers: []string{"user@example.com"},
    },
}
```

### Note

-   The `Content-ID` is generated from the **base filename** of the attachment (e.g., `filepath.Base("/path/to/assets/logo.png")` -> `logo.png`).
-   Ensure the filename in `cid:<filename>` matches exactly with the attached file's name.
