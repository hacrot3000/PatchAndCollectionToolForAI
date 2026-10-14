using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace TaskDeck.WindowsLauncher;

// Thin Windows UI only. TaskDeck's PTY/SSH/git engine runs inside Docker Linux.
internal static class Program
{
    [STAThread]
    private static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new LauncherForm());
    }
}
internal sealed class LauncherForm : Form
{
    private readonly TextBox project = new() { Width = 480 };
    private readonly TextBox image = new() { Width = 230, Text = "taskdeck:local" };
    private readonly NumericUpDown port = new() { Minimum = 1024, Maximum = 65535, Value = 42882, Width = 95 };
    private readonly ComboBox profile = new() { Width = 105, DropDownStyle = ComboBoxStyle.DropDownList };
    private readonly CheckBox pullImage = new() { Text = "Pull image (registry)", AutoSize = true };
    private readonly TextBox output = new() {
        Multiline = true, ReadOnly = true, ScrollBars = ScrollBars.Both, WordWrap = false,
        Font = new Font("Consolas", 9), Dock = DockStyle.Fill
    };
    private readonly Label status = new() { Text = "Select a Windows project folder and start Docker Desktop.", AutoSize = true };
    private readonly FlowLayoutPanel buttons = new() { AutoSize = true, WrapContents = true };
    private Process? child;

    public LauncherForm()
    {
        Text = "TaskDeck · Windows Docker Launcher";
        Width = 860;
        Height = 600;
        MinimumSize = new Size(680, 430);
        StartPosition = FormStartPosition.CenterScreen;
        project.Text = Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments);
        profile.Items.AddRange(new object[] { "runtime", "dev" });
        profile.SelectedIndex = 0;

        var layout = new TableLayoutPanel {
            Dock = DockStyle.Fill, Padding = new Padding(12), ColumnCount = 1, RowCount = 5
        };
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));

        var pathRow = new FlowLayoutPanel { AutoSize = true, WrapContents = true };
        pathRow.Controls.Add(new Label { Text = "Project folder:", AutoSize = true, Margin = new Padding(2, 9, 6, 0) });
        pathRow.Controls.Add(project);
        var browse = new Button { Text = "Browse…", AutoSize = true };
        browse.Click += (_, _) => {
            using var folder = new FolderBrowserDialog {
                Description = "Choose TaskDeck project on your Windows drive",
                UseDescriptionForTitle = true,
                SelectedPath = Directory.Exists(project.Text) ? project.Text : ""
            };
            if (folder.ShowDialog(this) == DialogResult.OK) project.Text = folder.SelectedPath;
        };
        pathRow.Controls.Add(browse);

        var configRow = new FlowLayoutPanel { AutoSize = true, WrapContents = true };
        configRow.Controls.Add(new Label { Text = "Image:", AutoSize = true, Margin = new Padding(2, 9, 5, 0) });
        configRow.Controls.Add(image);
        configRow.Controls.Add(new Label { Text = "Profile:", AutoSize = true, Margin = new Padding(8, 9, 5, 0) });
        configRow.Controls.Add(profile);
        configRow.Controls.Add(new Label { Text = "Port:", AutoSize = true, Margin = new Padding(8, 9, 5, 0) });
        configRow.Controls.Add(port);
        configRow.Controls.Add(pullImage);

        AddAction("Start", "Start");
        AddAction("Stop", "Stop");
        AddAction("Status", "Status");
        AddAction("Update", "Update");
        AddAction("Doctor", "Doctor");
        AddAction("Logs", "Logs");
        var open = new Button { Text = "Open Firefox/Browser", AutoSize = true };
        open.Click += (_, _) => {
            try {
                Process.Start(new ProcessStartInfo($"https://localhost:{port.Value}") { UseShellExecute = true });
            } catch (Exception ex) { MessageBox.Show(this, ex.Message, "Cannot open browser"); }
        };
        buttons.Controls.Add(open);

        var note = new Label {
            Text = "Backend: Linux container, not Windows-native ConPTY. Mount is limited to the chosen project. HTTPS + generated admin password; do not commit .vscode/vscode_tasks_menu.ini.",
            AutoSize = true, ForeColor = SystemColors.GrayText, MaximumSize = new Size(800, 0)
        };

        layout.Controls.Add(pathRow, 0, 0);
        layout.Controls.Add(configRow, 0, 1);
        layout.Controls.Add(buttons, 0, 2);
        layout.Controls.Add(note, 0, 3);
        layout.Controls.Add(output, 0, 4);
        Controls.Add(layout);
        var statusStrip = new StatusStrip();
        statusStrip.Items.Add(new ToolStripControlHost(status));
        statusStrip.Items.Add(new ToolStripStatusLabel { Text = "Docker Desktop + WSL2 backend" });
        Controls.Add(statusStrip);
        FormClosing += (_, e) => {
            if (child is { HasExited: false }) {
                if (MessageBox.Show(this, "Docker command is still running. Close launcher?", "Confirm",
                        MessageBoxButtons.YesNo) != DialogResult.Yes) {
                    e.Cancel = true;
                } else {
                    try { child.Kill(entireProcessTree: true); } catch { /* nothing */ }
                }
            }
        };
    }

    private void AddAction(string title, string action)
    {
        var button = new Button { Text = title, AutoSize = true };
        button.Click += async (_, _) => await ExecuteAsync(action);
        buttons.Controls.Add(button);
    }

    private void Append(string? text)
    {
        if (string.IsNullOrEmpty(text) || IsDisposed) return;
        if (InvokeRequired) {
            try { BeginInvoke(new Action(() => Append(text))); } catch (InvalidOperationException) { }
            return;
        }
        output.AppendText(text + Environment.NewLine);
    }

    private async Task ExecuteAsync(string action)
    {
        var script = Path.Combine(AppContext.BaseDirectory, "taskdeck.ps1");
        if (!File.Exists(script)) {
            MessageBox.Show(this, "taskdeck.ps1 must be located beside TaskDeckLauncher.exe.", "Missing script");
            return;
        }
        if (!Directory.Exists(project.Text)) {
            MessageBox.Show(this, "Choose an existing project directory.", "Invalid project");
            return;
        }
        foreach (Control item in buttons.Controls) item.Enabled = false;
        output.Clear();
        Append($"TaskDeck {action} · {project.Text}");
        try
        {
            var psi = new ProcessStartInfo("powershell.exe") {
                RedirectStandardOutput = true, RedirectStandardError = true,
                UseShellExecute = false, CreateNoWindow = true
            };
            foreach (var arg in new[] { "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
                     "-Action", action, "-ProjectPath", project.Text,
                     "-Profile", profile.Text, "-Image", image.Text,
                     "-Port", port.Value.ToString(), "-NoBrowser" }) {
                psi.ArgumentList.Add(arg);
            }
            if (pullImage.Checked) psi.ArgumentList.Add("-PullImage");
            using var process = new Process { StartInfo = psi };
            child = process;
            process.OutputDataReceived += (_, e) => Append(e.Data);
            process.ErrorDataReceived += (_, e) => Append(e.Data);
            if (!process.Start()) throw new InvalidOperationException("PowerShell process failed to start");
            process.BeginOutputReadLine();
            process.BeginErrorReadLine();
            await process.WaitForExitAsync();
            Append($"Exit code: {process.ExitCode}");
            if (process.ExitCode != 0) {
                status.Text = $"TaskDeck {action} failed (exit {process.ExitCode}).";
            } else {
                status.Text = $"TaskDeck {action} completed.";
            }
        }
        catch (Exception ex) { Append(ex.Message); status.Text = "Operation failed."; }
        finally {
            child = null;
            foreach (Control item in buttons.Controls) item.Enabled = true;
        }
    }
}
