# brainDB {{ current version of brainDB:version|kind=fact|section=facts }} release notes

Status: {{ project status concept not a stable contract:text|kind=fact|section=facts|min=0.3 }}

## Behaviour changes

{{#each decisions about defaults that change behaviour|kind=decision|section=decisions|limit=5|min=0.2}}
- {{ .id }}: {{ .text }}
{{/each}}

## Measured

- Measured with {{ offline engine of the recorded measurement:text|kind=benchmark|field=engine }} over {{ size of the brain measured:number|kind=benchmark|field=nodes }} nodes on {{ date of the recorded measurement:date|kind=benchmark|field=date }}
- Best mode overall: {{ which search mode ranks best overall:text|kind=benchmark|section=measurements|min=0.3 }}
- The failure to state plainly: {{ paraphrase queries that return nothing relevant in the top ten:text|kind=fact|section=facts|min=0.3 }}
- Still unmeasured: {{ latency and scale are not measured at all:text|kind=fact|section=facts|min=0.3 }}

## Open questions

{{#each open questions blocking the next release|kind=question|section=questions|limit=5|min=0.2}}
- {{ .id }}: {{ .text }}
{{/each}}

## Next

{{#each tasks planned for the next release|kind=task|section=tasks|limit=6|min=0.2}}
- {{ .text }}
{{/each}}

Decay policy in force: {{ what decay must never do to knowledge:text|kind=decision|section=decisions|min=0.3 }}
