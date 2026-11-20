# Deal review: {{ deal name|kind=deal }}

- **Account:** {{ client name:name|kind=account }}
- **Owner:** {{ deal owner:name|field=owner }} ({{ account owner email:email }})
- **CRM opportunity:** {{ CRM opportunity id:id|field=opportunity }}
- **Stage:** {{ deal stage|kind=deal|field=stage }}
- **Value:** {{ deal value:money|kind=deal }}
- **Close date:** {{ deal close date:date|kind=deal }}

## Forecast

Q4 target {{ Q4 new-bookings target:money }}, commit {{ Q4 commit forecast:money|kind=decision }}.
Forecast rule: {{ forecast category definition commit:text|kind=summary }}

## Pricing and discount

- Offer: {{ renewal offer term and price:text|kind=decision }}
- Discount: {{ multi-year discount:percent }}; approval rule: {{ discount approval rule above 10%:text|kind=decision }}
- Margin floor: {{ margin floor:percent }}

## Product claims to check with the lab

{{#each product claims and questions about NaCore 2|concept=product|limit=4}}
- {{ .id }}: {{ .text }}
{{/each}}

## Risks and open questions

{{#each open questions|kind=question|scope=context|limit=5}}
- {{ .text }} (weight {{ .weight }})
{{/each}}

## Competition

{{ competitor offer and price:text|concept=competition }}

## Next steps

{{#each next tasks|kind=task|scope=context|limit=5}}
- [ ] {{ .text }}
{{/each}}
