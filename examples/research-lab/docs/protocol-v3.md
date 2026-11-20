# Cell test protocol v3 (Corvane Institute, Electrolyte Group)

Version 3, effective 30 Oct 2026. Replaces protocol v2 (formation 2 cycles at C/20, cycling at 1C).
Owner: Dr. Hanne Solberg (PI). Approved for all EL-series electrolyte tests.
Online copy: https://corvane.example/protocols/v3

## Electrolyte preparation

1. Dry NaPF6 at 80 °C under vacuum for 12 h.
2. Mix in the argon glovebox (O2 and H2O below 5 ppm and 1 ppm).
3. Measure water by Karl Fischer titration; accept the batch only below 20 ppm.
4. Label the bottle with the sample id (S-xxxx), batch and date; use within 21 days.

Baseline EL-0 is 1.0 M NaPF6 in EC:DEC 1:1 by volume. EL-7 adds 2 wt% FEC and 0.5 wt% TMSB.

## Cell build

- Coin cells CR2032: hard carbon HC-3 anode, NVPF cathode, 2 mAh/cm2 loading, glass fibre separator, 80 µL electrolyte.
- Pouch cells: 200 mAh single-layer, 1.2 g electrolyte, vacuum sealed at 165 °C.
- Minimum n=4 replicate cells per condition. Report mean ± standard deviation.

## Formation

Three cycles at C/10 between 2.0 and 4.3 V, with a 12 h rest at open circuit before the first charge.
Record first-cycle Coulombic efficiency.

## Cycling

- C/2 charge and discharge between 2.0 and 4.3 V at 25 °C (standard) or 45 °C (high temperature).
- Electrochemical impedance spectroscopy (EIS) every 50 cycles, 100 kHz to 10 mHz, 10 mV amplitude.
- Do not use cycler channel 14 for EIS (noisy).
- End of life: 80% of the capacity at cycle 5.

## Gas measurement (pouch cells)

Archimedes method in silicone oil, before and after formation. Check the seal on every pouch
before measuring: a leaking seal gave a false 0.9 mL reading in October.

## Low temperature

Capacity at −20 °C at C/5 after 4 h soak, reported as a percentage of the 25 °C capacity.

## Data

Export cycler data with the run id (RUN-xxxx), convert to parquet, run the QC notebook, then upload
to the lab data store. Every number in a report cites the run id and the data file.
