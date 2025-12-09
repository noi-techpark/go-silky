# IMMEDIATE SETUP STEPS

Follow these steps **exactly** in order:

## Step 1: Check Your File

```bash
cd /home/mroggia/git/go-silky/vscode-extension
ls -la *.yaml *.yml 2>/dev/null
```

**Expected:** You should see files ending in `.silky.yaml`

**If you see `example.yaml`:** Rename it:
```bash
mv example.yaml example.silky.yaml
```

## Step 2: Reload Extension Development Host

In the Extension Development Host window (the one that opened when you pressed F5):

Press **`Ctrl+Shift+P`** → Type **"Developer: Reload Window"** → Press Enter

OR just press **`Ctrl+R`**

## Step 3: Install YAML Extension

1. In the Extension Development Host, press **`Ctrl+Shift+X`** (Extensions view)
2. Search for **"YAML"**
3. Install **"YAML"** by **Red Hat**
4. After installing, reload window again (**`Ctrl+R`**)

## Step 4: Open Your File

1. In Extension Development Host: **File → Open File**
2. Navigate to: `/home/mroggia/git/go-silky/vscode-extension/example.silky.yaml`
3. Click **Open**

## Step 5: Verify It Works

You should NOW see:

✅ **YAML Syntax Highlighting** - Colors on keywords, strings, etc.
✅ **Three buttons at top-right**: ▶ Run, ⏹ Stop, ✓ Validate
✅ **IntelliSense works** - Press `Ctrl+Space` to see suggestions
✅ **Snippets work** - Type `agr-basic` and press Tab

## Step 6: Test Run

1. Click the **▶ Run** button at top-right
2. **Open Output Panel**: View → Output → Select "Silky" from dropdown
3. **Check for success message**:
   - Should say: `Using bundled binary: /home/mroggia/git/go-silky/vscode-extension/bin/silky`
   - Should NOT say: `terminal not cursor addressable`
   - Should NOT say: `Could not find cmd/ide`

## Troubleshooting

### Still No YAML Highlighting?

**Check language in status bar** (bottom-right of VSCode):
- If it says `"Plain Text"` or `"silky-yaml"` → **Wrong!**
- Should say `"YAML"` → **Correct!**

**Force change to YAML**:
1. Click the language in status bar (bottom-right)
2. Type "YAML" and select it
3. File should now have colors

### Still Can't Find Binary?

```bash
# Verify binary exists:
ls -lh /home/mroggia/git/go-silky/vscode-extension/bin/silky

# Should show:
# lrwxrwxrwx ... silky -> silky-linux-amd64
```

**If missing, rebuild**:
```bash
cd vscode-extension
npm run build:binary
```

### Buttons Not Showing?

**Check file extension**:
```bash
# In terminal:
basename /path/to/your/file
# Must end with: .silky.yaml or .silky.yml
```

**Fix if needed**:
- Use File Explorer in VSCode → Right-click file → Rename
- Change extension to `.silky.yaml`

## Expected Final Result

After following all steps, opening `example.silky.yaml` should show:

```
┌─────────────────────────────────────────────┐
│ example.silky.yaml          ▶ ⏹ ✓    │  ← These 3 buttons
├─────────────────────────────────────────────┤
│  1  # Example Silky Configuration           │  ← Blue comment
│  2  rootContext: []                         │  ← Syntax colors
│  3                                          │
│  4  steps:                                  │  ← Yellow keyword
│  5    - type: request                       │  ← Green string
│  6      url: "https://..."                  │  ← Orange URL
└─────────────────────────────────────────────┘
```

**Status bar should show**: `YAML` (not Plain Text, not silky-yaml)

## What Changed to Fix Issues

1. **Removed custom `silky-yaml` language ID** - Was blocking YAML extension
2. **Added automatic language detection** - Forces files to use YAML language
3. **Created symlink for binary** - `bin/silky` points to platform binary
4. **Fixed validation command** - Uses bundled binary instead of `go run`

## If Still Not Working

**Get diagnostic info**:
1. Press `Ctrl+Shift+P`
2. Type "Developer: Show Running Extensions"
3. Check if these are active:
   - ✅ `redhat.vscode-yaml` (YAML extension)
   - ✅ `noi-techpark.silky-vscode` (this extension)

**Check Output panel**:
- View → Output
- Select "Extension Host" from dropdown
- Look for any errors mentioning "silky" or "yaml"

**Last resort - Clean reload**:
```bash
# Close Extension Development Host window
# In main VSCode window:
# Ctrl+Shift+P → "Developer: Reload Window"
# Press F5 again to relaunch
```

---

## Success Criteria Checklist

- [ ] File ends with `.silky.yaml`
- [ ] Status bar shows "YAML" language
- [ ] File has syntax highlighting (colors)
- [ ] Three buttons (▶ ⏹ ✓) visible at top-right
- [ ] `Ctrl+Space` shows IntelliSense suggestions
- [ ] Typing `agr-basic` + Tab inserts snippet
- [ ] Clicking ▶ Run shows "Using bundled binary" in Output
- [ ] No "terminal not cursor addressable" error
- [ ] No "Could not find cmd/ide" error

**ALL checkboxes checked? → ✅ Extension is working!**
