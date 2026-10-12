# ADR（Architecture Decision Records）

## Rules

- [MADR（Markdown Any Decision Records）template](https://github.com/architecture-decision-record/architecture-decision-record/tree/main/locales/ja/%E3%83%86%E3%83%B3%E3%83%97%E3%83%AC%E3%83%BC%E3%83%88/madr%E3%83%97%E3%83%AD%E3%82%B8%E3%82%A7%E3%82%AF%E3%83%88%E3%81%AE%E6%84%8F%E6%80%9D%E6%B1%BA%E5%AE%9A%E8%A8%98%E9%8C%B2%E3%83%86%E3%83%B3%E3%83%97%E3%83%AC%E3%83%BC%E3%83%88) に従うこと
- Write contents Simple and short.
- Issue a serial number (`0001-`, `0002-`, …) for a new file.
- Write metadata as frontmatter.
- Write a title as H1.

## frontmatter

```yaml
---
status: <proposed | declined | approved | deprecated | superseded>
date: <YYYY-MM-DD>
deciders:
  - <@account name>
---
```
