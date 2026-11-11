@echo off
rem Starts all of Money 24. Double-click it, or run it from a terminal. Ctrl+C stops everything.
cd /d "%~dp0"
where node >nul 2>nul || (echo Node.js is not installed. Get it from https://nodejs.org/ then run this again. & pause & exit /b 1)
where go >nul 2>nul || (echo Go is not installed. Get it from https://go.dev/dl/ then run this again. & pause & exit /b 1)
if not exist node_modules (call npm install || (echo npm install failed. & pause & exit /b 1))
node scripts\start.js --open
pause
