---
title: "FBS Interlock Gateway"
subtitle: "Shelly Interlock Hardware Guide"
author: "William Veith"
date: "2026-08-09"
lang: en-US
---

> **Purpose**
>
> This guide documents the wiring configurations, junction-box label artwork, material selections, device-identification format, and fabrication notes for the Shelly-based interlock hardware used with `fbs-interlock-gateway-cluster`.

> **Security and safety boundary**
>
> The gateway and Shelly relay control software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. This guide does not replace qualified electrical review, applicable electrical codes, equipment ratings, lockout/tagout procedures, or facility safety requirements. Verify the terminal layout and ratings printed on the exact Shelly model before wiring it.

# Table of Contents

- [Hardware Overview](#hardware-overview)
- [Supported Power Configurations](#supported-power-configurations)
- [Pre-Fabrication Checklist](#pre-fabrication-checklist)
- [General Materials](#general-materials)
- [Bill of Materials](#bill-of-materials)
- [Wiring Configuration: External 12 VDC Supply](#wiring-configuration-external-12-vdc-supply)
- [Wiring Configuration: Tool 110–240 VAC Supply](#wiring-configuration-tool-110240-vac-supply)
- [Junction-Box Labels](#junction-box-labels)
- [QR-Code Device Identity](#qr-code-device-identity)
- [Fiber-Laser Marking Parameters](#fiber-laser-marking-parameters)
- [Assembly and Documentation Workflow](#assembly-and-documentation-workflow)
- [Verification Checklist](#verification-checklist)
- [Replacement and Maintenance](#replacement-and-maintenance)
- [File Reference](#file-reference)

<div class="page-break"></div>

# Hardware Overview

The interlock hardware consists of a Shelly 1 relay installed in a labeled junction box and wired between the facility gateway system and the tool's interlock or monitor circuit.

The repository maintains the hardware design files under:

```text
docs/hardware/
```

The directory contains:

- two supported wiring configurations
- junction-box label artwork for each power configuration
- general construction materials
- a serialized QR-code device identity format
- recorded fiber-laser marking settings

The two supported power arrangements are:

1. an interlock box powered from an external 12 VDC supply
2. an interlock box powered from the tool's 110–240 VAC supply

```text
FBS
    -> fbs-interlock-gateway-cluster
    -> Shelly RPC over HTTP or HTTPS
    -> Shelly relay dry contact
    -> tool interlock / monitor circuit
```

The software, enclosure, wiring, and serialized device label together form one auditable interlock assembly.

# Supported Power Configurations

## External 12 VDC supply

The Shelly is powered from a separate 12 VDC source. The relay's isolated dry contacts switch the tool interlock circuit.

Use the diagram:

```text
Wiring Powered From 12VDC.svg
```

## Tool 110–240 VAC supply

The Shelly is powered from the tool's 110–240 VAC supply. The relay output is used for the tool interlock connection.

Use the diagram:

```text
Wiring Powered From Tool (110-240VAC).svg
```

> **Important**
>
> Do not assume that terminal names, current ratings, voltage ratings, or power-input options are identical across Shelly generations or regional variants. Confirm the markings and documentation for the exact installed model before applying power.

# Pre-Fabrication Checklist

Before constructing or labeling an interlock box, confirm the following:

- The required power configuration has been selected.
- The exact Shelly generation and model number are known.
- The Shelly terminal arrangement has been checked against the selected wiring diagram.
- The tool interlock circuit requirements are understood.
- The junction box has adequate internal clearance for the relay, connectors, wiring, and cable glands.
- The cable glands match the intended cable diameters and environmental requirements.
- The correct junction-box label artwork has been selected.
- The QR code contains the correct device name, model number, and unique Shelly ID.
- No password, certificate, private key, network credential, or other secret is present in the QR data.
- The enclosure lid material and coating have been test-marked before production engraving.
- Electrical work will be performed under the facility's required review and lockout/tagout procedures.

# General Materials

| Component | Specification |
| --- | --- |
| Junction box | Black ABS; nominal size 3.5 W × 4.5 H × 2.2 D; IP65 |
| Cable glands | Black nylon PA66; IP68 |
| Wireless relay | Shelly 1 Gen4 or Shelly 1 Gen3 |
| Two-conductor connector | WAGO 221-412 lever-nut splicing connector |
| Three-conductor connector | WAGO 221-413 lever-nut splicing connector |

The enclosure size is recorded as supplied for the selected box. Confirm the manufacturer's dimensional units, internal dimensions, lid thickness, cable-entry clearances, and available mounting space before fabrication.

> **Material substitution**
>
> When substituting an enclosure, cable gland, connector, or relay model, verify that the replacement remains suitable for the intended voltage, conductor type, environment, and installation method. Update the corresponding drawing or build record when the substitution changes the assembly.

# Bill of Materials

The following bills of materials are based on the `Materials Required For Junction Boxes` workbook. Quantities and costs are per completed junction-box assembly. The unit-price values link to the specific product listings recorded in the workbook.

> **Pricing note**
>
> Unit prices are procurement references, not fixed quotations. Prices, package quantities, shipping, taxes, and product availability may change. Line costs and assembly totals below use the quantities and unit prices recorded in the source workbook; displayed totals are rounded to the nearest cent.

## External 12 VDC supply

| Component | Specification | Qty. | Unit Price | Line Cost |
| --- | --- | ---: | ---: | ---: |
| Junction Box | 4.5" × 3.5" × 2.2" IP65 ABS, black | 1 | [$7.25](https://www.amazon.com/Zulkit-Dustproof-Waterproof-Universal-Electrical/dp/B07Z42YCXF?refinements=p_n_g-101013999996111%3A107518270011%2Cp_n_g-101013998791111%3A107245770011%2Cp_n_g-101013998994111%3A107245762011&rnid=107245654011) | $7.25 |
| Cable Gland | PG7, IP68, PA66, black | 2 | [$0.45](https://www.amazon.com/AMPELE-Plastic-Waterproof-Adjustable-Connectors/dp/B08TCFM4C7) | $0.90 |
| Wireless Relay | Shelly 1 Gen4 (S4SW-001X16EU) | 1 | [$24.99](https://us.shelly.com/products/shelly-1-gen4) | $24.99 |
| Power Supply | 12 VDC, 10' cable | 1 | [$10.36](https://www.amazon.com/Adapter-Extension-5-5x2-5mm-Replacement-Transformer/dp/B0D4TBSZ4B?cv_ct_cx=12v%2B1a%2Bpower%2Bsupply&refinements=p_n_g-1004150843091%3A23555329011&rnid=23555276011&aref=Vxpa8WQnsK) | $10.36 |
| Splicing Connector | WAGO 221-412, 600 V, 105 °C, V2 | 1 | [$0.34](https://www.homedepot.com/pep/WAGO-221-412-Wire-Lever-Nuts-2-Wire-Conductor-Compact-Splicing-Connectors-50-Pack-0221412K00-012/326254029?mtc=SEM-BF-CDP-GGL-D27E-027_011_ELECTRICAL_ACCESSORIES-NA-NA-NA-DSA-NA-NA-NA-NA-NBR-NA-NA-NEW-NA-Feed&cm_mmc=SEM-BF-CDP-GGL-D27E-027_011_ELECTRICAL_ACCESSORIES-NA-NA-NA-DSA-NA-NA-NA-NA-NBR-NA-NA-NEW-NA-Feed-16187052617-134167688558-2399809273423&gclsrc=aw.ds&gad_source=1&gad_campaignid=16187052617&gbraid=0AAAAADq61UeUSmuKELQSO79cdjPO_zOLu&gclid=Cj0KCQjwsMLSBhD9ARIsAIpUTDqQDcs5WrQ3FeE08Gp0L2s61zYMUIUL3y2XW9-_mob8DLrPHo_fDFEaAsumEALw_wcB) | $0.34 |
| Ferrule | 12 AWG insulated bootlace ferrule, single-conductor, black collar, 9 mm pin, 3.2 mm barrel OD, 105 °C | 4 | [$0.01](https://www.amazon.com/Fidioto-Terminals-Connector-Insulated-Industrial/dp/B0BFFPGFYD) | $0.04 |

**Estimated material cost per 12 VDC assembly: $43.88**

## Tool 110–240 VAC supply

| Component | Specification | Qty. | Unit Price | Line Cost |
| --- | --- | ---: | ---: | ---: |
| Junction Box | 4.5" × 3.5" × 2.2" IP65 ABS, black | 1 | [$7.25](https://www.amazon.com/Zulkit-Dustproof-Waterproof-Universal-Electrical/dp/B07Z42YCXF?refinements=p_n_g-101013999996111%3A107518270011%2Cp_n_g-101013998791111%3A107245770011%2Cp_n_g-101013998994111%3A107245762011&rnid=107245654011) | $7.25 |
| Cable Gland | PG7, IP68, PA66, black | 2 | [$0.45](https://www.amazon.com/AMPELE-Plastic-Waterproof-Adjustable-Connectors/dp/B08TCFM4C7) | $0.90 |
| Wireless Relay | Shelly 1 Gen4 (S4SW-001X16EU) | 1 | [$24.99](https://us.shelly.com/products/shelly-1-gen4) | $24.99 |
| Splicing Connector | WAGO 221-412, 600 V, 105 °C, V2 | 2 | [$0.34](https://www.homedepot.com/pep/WAGO-221-412-Wire-Lever-Nuts-2-Wire-Conductor-Compact-Splicing-Connectors-50-Pack-0221412K00-012/326254029?mtc=SEM-BF-CDP-GGL-D27E-027_011_ELECTRICAL_ACCESSORIES-NA-NA-NA-DSA-NA-NA-NA-NA-NBR-NA-NA-NEW-NA-Feed&cm_mmc=SEM-BF-CDP-GGL-D27E-027_011_ELECTRICAL_ACCESSORIES-NA-NA-NA-DSA-NA-NA-NA-NA-NBR-NA-NA-NEW-NA-Feed-16187052617-134167688558-2399809273423&gclsrc=aw.ds&gad_source=1&gad_campaignid=16187052617&gbraid=0AAAAADq61UeUSmuKELQSO79cdjPO_zOLu&gclid=Cj0KCQjwsMLSBhD9ARIsAIpUTDqQDcs5WrQ3FeE08Gp0L2s61zYMUIUL3y2XW9-_mob8DLrPHo_fDFEaAsumEALw_wcB) | $0.68 |
| Splicing Connector | WAGO 221-413, 600 V, 105 °C, V2 | 2 | [$0.4294](https://www.homedepot.com/p/WAGO-221-413-Lever-Nuts-3-Conductor-Splicing-Connectors-12-24AWG-50-Pack-0221413K00-012/326483207?MERCH=REC-_-pipsem-_-326254029-_-0-_-n/a-_-n/a-_-n/a-_-n/a-_-n/a) | $0.86 |
| Wire | 12 AWG stranded copper, UL 1015, 600 V, 105 °C, VW-1, white | 1 | [$0.07](https://www.amazon.com/Stranded-Electrical-Insulation-Residential-Industrial/dp/B0G13HX87C) | $0.07 |
| Wire | 12 AWG stranded copper, UL 1015, 600 V, 105 °C, VW-1, black | 2 | [$0.07](https://www.amazon.com/Stranded-Electrical-Insulation-Residential-Industrial/dp/B0G13PJVXY) | $0.14 |
| Ferrule | 12 AWG insulated bootlace ferrule, single-conductor, 9 mm pin, 3.2 mm barrel OD, 105 °C | 4 | [$0.01](https://www.amazon.com/Fidioto-Terminals-Connector-Insulated-Industrial/dp/B0BFFPGFYD) | $0.04 |

**Estimated material cost per 110–240 VAC assembly: $34.93**

<div class="page-break"></div>

# Wiring Configuration: External 12 VDC Supply

This configuration powers the Shelly from a separate 12 VDC source while using the relay's dry contacts to switch the tool interlock circuit.

<img src="assets/Wiring Powered From 12VDC.svg" width="450">

## Design intent

- The external 12 VDC supply powers the Shelly.
- The Shelly relay contacts remain electrically separate from the Shelly power input.
- The relay contact pair is connected to the tool interlock or monitor circuit according to the equipment requirements.
- The selected enclosure label identifies the 12 VDC power arrangement and the external connections.

## Required drawing

```text
docs/hardware/Wiring Powered From 12VDC.svg
```

## Matching label artwork

```text
docs/hardware/12VDC Junction Box Label.svg
```

> **Verification**
>
> Confirm polarity, conductor routing, terminal identification, and the exact dry-contact terminals on the installed Shelly before energizing the assembly.

# Wiring Configuration: Tool 110–240 VAC Supply

This configuration powers the Shelly from the tool's 110–240 VAC supply while using the relay output for the tool interlock connection.

<img src="assets/Wiring Powered From Tool (110-240VAC).svg" width="450">

## Design intent

- The Shelly receives operating power from the tool's AC supply.
- The relay output connects to the tool interlock circuit according to the equipment requirements.
- The selected enclosure label identifies the 110–240 VAC power arrangement and the external connections.

## Required drawing

```text
docs/hardware/Wiring Powered From Tool (110-240VAC).svg
```

## Matching label artwork

```text
docs/hardware/110-240VAC Junction Box Label.svg
```

> **Line-voltage caution**
>
> The 110–240 VAC configuration includes line-voltage wiring. Construction, installation, inspection, and maintenance must follow the applicable facility electrical requirements and lockout/tagout procedures.

# Junction-Box Labels

The junction-box lid artwork identifies the selected power configuration and external connections.

## 12 VDC label

<img src="assets/12VDC Junction Box Label.svg" width="450">

Source file:

```text
docs/hardware/12VDC Junction Box Label.svg
```

## 110–240 VAC label

<img src="assets/110-240VAC Junction Box Label.svg" width="450">

Source file:

```text
docs/hardware/110-240VAC Junction Box Label.svg
```

## Label selection rules

- Use the label that matches the actual power configuration.
- Confirm the printed connection names match the completed wiring.
- Generate the QR code from the Shelly installed in that specific enclosure.
- Replace or remark the lid when the relay identity or power configuration changes.
- Do not reuse a serialized lid on an enclosure containing a different Shelly.

<div class="page-break"></div>

# QR-Code Device Identity

Each junction-box label includes a QR code containing serialized JSON that identifies the installed Shelly relay.

The record uses three fields:

| Field | Purpose |
| --- | --- |
| `device` | Human-readable Shelly product name |
| `model` | Shelly hardware model number |
| `id` | Unique Shelly device ID |

Example:

```json
{"device": "Shelly 1 Gen4", "model": "S4SW-001X16EU", "id": "A085E3B5325C"}
```

## Audit purpose

Scanning the label with a phone provides a fast, auditable method for collecting the installed device identity during:

- initial installation
- inspection
- preventive maintenance
- inventory reconciliation
- troubleshooting
- certificate or hostname verification
- hardware replacement

The scanned JSON can be copied directly into an inventory, service, or installation record without manually transcribing the product name, model number, and device ID.

## Data rules

Keep the field names stable:

```text
device
model
id
```

Stable field names allow the QR data to be parsed consistently by future inventory or audit tools.

The QR record must:

- describe the Shelly installed in that enclosure
- use the exact hardware model number
- use the unique Shelly ID
- be regenerated whenever the installed Shelly is replaced

The QR record must not contain:

- passwords
- certificates
- private keys
- Wi-Fi credentials
- Digest credentials
- network secrets
- other sensitive configuration data

## Replacement rule

When the Shelly is replaced:

1. Record the outgoing device identity.
2. Install and configure the replacement Shelly.
3. Generate a new JSON identity record.
4. Generate a new QR code.
5. Update or replace the junction-box label.
6. Confirm the scanned label matches the installed device.

# Fiber-Laser Marking Parameters

The example lid labels were produced with a 30 W fiber laser using the following recorded settings:

| Parameter | Setting |
| --- | ---: |
| Speed | 60% |
| Power | 30% |
| Frequency | 80 |

These are machine settings, not universal material-processing specifications.

Before marking a production enclosure:

- verify laser focus
- verify the marking area
- confirm the enclosure coating response
- confirm ventilation and fume-control requirements
- test the artwork on a spare lid
- verify that text and the QR code remain readable
- scan the QR code from the finished test mark
- confirm that the marked lid is still mechanically usable

> **QR-code verification**
>
> A visually acceptable QR code is not sufficient. Scan the completed mark with a phone and confirm that the decoded JSON exactly matches the installed Shelly before placing the enclosure into service.

# Assembly and Documentation Workflow

Use the following sequence for each interlock box.

1. Select the required 12 VDC or 110–240 VAC power configuration.
2. Confirm the exact Shelly device name, model number, and unique device ID.
3. Select the matching wiring diagram.
4. Select the matching junction-box label artwork.
5. Confirm enclosure and cable-gland fit.
6. Install the cable glands and internal components.
7. Complete the wiring under the required electrical controls.
8. Generate the serialized JSON identity record.
9. Generate and place the matching QR code in the label artwork.
10. Mark the enclosure lid.
11. Scan and verify the finished QR code.
12. Inspect the completed wiring against the drawing.
13. Record the device identity and installation information in the applicable inventory or service record.
14. Perform authorized functional verification before placing the interlock into service.

# Verification Checklist

## Mechanical inspection

- The enclosure and lid are undamaged.
- Cable glands are installed securely.
- Conductors are routed without excessive strain.
- Lever connectors are fully closed.
- No exposed conductor extends beyond the intended terminal or connector.
- Internal parts do not interfere with the lid.
- The label is legible and corresponds to the actual power configuration.

## Electrical inspection

- The selected diagram matches the completed wiring.
- The exact Shelly terminal markings have been verified.
- The power source matches the selected configuration.
- The tool interlock conductors are connected to the intended relay contacts.
- Required facility electrical inspection has been completed.
- Lockout/tagout controls were used as required.
- Energization and functional tests are authorized.

## Identity and audit inspection

- The QR code scans successfully.
- The decoded data is valid JSON.
- The `device` value matches the installed product name.
- The `model` value matches the installed hardware model.
- The `id` value matches the unique Shelly ID.
- The identity record has been entered into the applicable inventory or service record.
- No secrets are encoded in the QR data.

# Replacement and Maintenance

When servicing an existing interlock box:

- scan and record the installed Shelly identity before disassembly
- confirm the existing wiring configuration
- apply the required lockout/tagout procedure
- inspect the enclosure, glands, connectors, and conductors
- replace damaged or unsuitable components
- verify the wiring against the current repository drawing
- preserve the correct power-configuration label
- regenerate the QR code whenever the Shelly changes
- scan the completed label before returning the box to service
- update the maintenance or inventory record

A wiring or identity change should be treated as a documented configuration change, not as an undocumented field modification.

# File Reference

The authoritative editable hardware assets are:

```text
docs/hardware/
├── Shelly Interlock Hardware Guide.md
└── assets /
    ├── 110-240VAC Junction Box Label.svg
    ├── 12VDC Junction Box Label.svg
    ├── Wiring Powered From 12VDC.svg
    └── Wiring Powered From Tool (110-240VAC).svg
```

| File | Purpose |
| --- | --- |
| `Wiring Powered From 12VDC.svg` | Wiring diagram for an externally powered 12 VDC Shelly interlock box |
| `Wiring Powered From Tool (110-240VAC).svg` | Wiring diagram for a Shelly interlock box powered from the tool's AC supply |
| `12VDC Junction Box Label.svg` | Lid artwork for the 12 VDC configuration |
| `110-240VAC Junction Box Label.svg` | Lid artwork for the 110–240 VAC configuration |

