# Experiment report: {{ experiment run id:id|kind=experiment|field=run_id }}

- **Condition:** {{ experiment condition|kind=experiment|field=condition }}
- **Temperature:** {{ test temperature in °C:number|kind=experiment|field=temperature_c }} °C
- **Operator:** {{ experiment operator:name|kind=experiment|field=operator }}
- **Start date:** {{ experiment start date:date|kind=experiment|field=start_date }}
- **Protocol:** {{ current test protocol version and formation steps:text|kind=decision }}

## Electrolyte

- Baseline: {{ baseline electrolyte composition:text }}
- Candidate: {{ EL-7 additive composition:text }}
- Batch in use: {{ electrolyte batch sample id:id|kind=sample|field=sample_id }}, water {{ batch water content ppm:number|kind=sample|field=water_ppm }} ppm

## Results

- Capacity retention at 25 °C: {{ capacity retention after 200 cycles at 25 °C:percent }}
- Capacity retention at 45 °C: {{ capacity retention after 150 cycles at 45 °C:percent }}
- First-cycle Coulombic efficiency: {{ first-cycle Coulombic efficiency EL-7:percent }}
- Pouch formation gas: {{ pouch gas volume after formation:text }}
- Ionic conductivity: {{ ionic conductivity at 25 °C:text }}

{{#each measurement records for this run|section=measurements|limit=8}}
| {{ .fields.cell_id }} | {{ .fields.electrolyte }} | {{ .fields.temperature_c }} °C | {{ .fields.capacity_retention_pct }} % | {{ .fields.run_date }} |
{{/each}}

## Hypotheses

{{#each hypotheses being tested|kind=hypothesis|limit=3}}
- {{ .text }}
{{/each}}

## Open questions

{{#each open questions|kind=question|scope=context|limit=4}}
- {{ .text }}
{{/each}}

Decision deadline: {{ go/no-go decision date for pouch validation:date|kind=decision }}.
