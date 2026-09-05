import pathlib,sys
p=pathlib.Path(__file__).parent/'apply.py'
s=p.read_text()
s=s.replace("edit(p,'s.registerAutopilotRoutes(mux)','s.registerAutopilotRoutes(mux)\\n\\ts.registerAuthoringRoutes(mux)')", "edit(p,'s.registerWorkspaceRoutes(mux)','s.registerWorkspaceRoutes(mux)\\n\\ts.registerAuthoringRoutes(mux)')")
start=s.index("p='web/src/lib/router.ts'")
end=s.index('# API methods',start)
s=s[:start]+'''p='web/src/lib/router.ts'
edit(p,"RouteName = 'autopilot'", "RouteName = 'authoring' | 'autopilot'")
edit(p,"  '/autopilot': 'autopilot',", "  '/autopilot': 'autopilot',\\n  '/authoring': 'authoring',")
'''+s[end:]
exec(compile(s,str(p),'exec'))
