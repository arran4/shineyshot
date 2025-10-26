//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

func psQuote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}

// Notify displays a toast notification using the Windows notification center.
func Notify(title, body string, opts Options) error {
	icon := strings.TrimSpace(opts.IconPath)
	var script string
	if icon == "" {
		script = fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=Windows Runtime] > $null; `+
			`$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02); `+
			`$texts = $template.GetElementsByTagName("text"); `+
			`$texts.Item(0).AppendChild($template.CreateTextNode(%s)) > $null; `+
			`$texts.Item(1).AppendChild($template.CreateTextNode(%s)) > $null; `+
			`$toast = [Windows.UI.Notifications.ToastNotification]::new($template); `+
			`$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(%s); `+
			`$notifier.Show($toast);`, psQuote(title), psQuote(body), psQuote("ShineyShot"))
	} else {
		script = fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=Windows Runtime] > $null; `+
			`$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastImageAndText02); `+
			`$texts = $template.GetElementsByTagName("text"); `+
			`$texts.Item(0).AppendChild($template.CreateTextNode(%s)) > $null; `+
			`$texts.Item(1).AppendChild($template.CreateTextNode(%s)) > $null; `+
			`$image = $template.GetElementsByTagName("image").Item(0); `+
			`$image.SetAttribute("src", %s); `+
			`$toast = [Windows.UI.Notifications.ToastNotification]::new($template); `+
			`$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(%s); `+
			`$notifier.Show($toast);`, psQuote(title), psQuote(body), psQuote(icon), psQuote("ShineyShot"))
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", script)
	return cmd.Run()
}
