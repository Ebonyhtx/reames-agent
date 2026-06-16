# Reames Agent
> AI coding agent, forked and customized from Hermes Agent

## Install

### Windows (one command)
```powershell
irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/install.ps1 | iex
```

### Manual install
```bash
git clone https://github.com/Ebonyhtx/reames-agent
cd reames-agent
python -m venv .venv
.venv/Scripts/activate
pip install -e .
setx DEEPSEEK_API_KEY sk-your-key
reames
```
