@echo off
mkdir "%~dp0\chromeData"
start msedge --remote-debugging-port=9222 --user-data-dir="%~dp0\msedgeData"
pause