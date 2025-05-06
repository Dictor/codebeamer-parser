@echo off
mkdir "%~dp0\chromeData"
start chrome --remote-debugging-port=9222 --user-data-dir="%~dp0\chromeData"
pause