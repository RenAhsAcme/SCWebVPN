# SCWebVPN

[简体中文](README.md) | English

Quickly penetrate intranets anytime, anywhere using a web browser.

> [!CAUTION]
> This project is still in draft form. Some documentation may not accurately reflect reality, and it contains a significant amount of AIGC that has not been manually reviewed. Do not deploy this project in any production environment. For previewing and development, this project includes documentation for the AI Agent, allowing you to quickly get started using AI.

## Feature Introduction

SCWebVPN originated from my needs during my university life. Sometimes when attending practical courses in the computer lab, the computers often had restore protection, and the tools on those computers obviously didn't meet my needs. Additionally, sometimes I deployed services on the intranet and wanted to access them remotely. Installing remote control software on-site wasn't convenient, so I conceived the idea of ​​creating a WebVPN for intranet penetration services with zero installation cost.

## Prerequisites

This project is based on the following conditions and was developed with deep AI involvement; therefore, if your existing conditions do not meet the following conditions, some unexpected performance results may occur.

- OpenWrt (x86_64);

- The entire network structure is visible under OpenWrt;

- A website that is publicly accessible, bound to a valid domain name, and protected by CDN;

- The network environment does not have symmetric NAT, UDP hole punching disabled, or other obstacles to intranet penetration.

> Due to my limited time and energy, I have not yet maintained a backup solution for intranet penetration failures. This project is practically unusable for you if your network environment cannot successfully penetrate.

## How to Get Started

When you clone this repository to your local machine, you will see [README_Local.md](../README_Local.md) in the repository root directory. This document contains a quick start guide that you can use for deployment.

For more detailed usage information, please visit the [GitHub Repository Wiki](https://github.com/RenAhsAcme/SCWebVPN/wiki).

## How to Contribute

- If your issue is a Bug/Security issue, the author will fix it as quickly as possible within their capabilities.

- If your issue is a Feature issue, it may take the author a considerable amount of time to implement.

- You can submit a Pull Request to request modifications to this project, following the community's established guidelines.

## License and Other Compliance Notices

Except for individual component licenses, the source code of this project is licensed under [GPL-3.0](../LICENSE).

This project contains components licensed under AGPL-3.0-only. Therefore, please consider the relevant terms and conditions when deploying related services.

The related text documentation for this project is licensed under the following license:

<a href="../LICENSE-CC">
  Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International
</a>

In Chinese Mainland, deploying this service for large-scale public use is prohibited. You must take appropriate measures to limit the service to a small group of people, including yourself and those you authorize.

It is prohibited to use this project for activities that violate any laws or regulations of the relevant government or organization, or that interfere with the normal operation of public networks.

The original author of the project is not responsible for any illegal activities involving the use or modification of this project.

## Support the Author

Click the Sponsor button on the repository homepage to be redirected to the donation page.
