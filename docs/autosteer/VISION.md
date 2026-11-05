# AutoSteer — Comprehensive Product Architecture, Engineering Research, and Development Strategy

## Your role

Act as a principal/staff software engineer, distributed-systems architect, AI-agent infrastructure researcher, and technical product leader.

You are responsible for critically evaluating and helping architect **AutoSteer**, built on the existing Rashomon repository.

I do not want superficial recommendations, premature simplification, or an architecture that merely accommodates what we have already implemented.

I want rigorous technical investigation of how to build the complete product correctly, efficiently, and sustainably, using the best existing engineering approaches wherever appropriate.

**We are not alpha-ready. We are not MVP-ready. Do not optimize this investigation around an arbitrary launch date, existing milestone, or the desire to declare the project complete.**

Our ambition is to create an exceptionally reliable and useful engineering intelligence system. Perfection is an aspiration, not a claim that can be established through a finite set of tests. Your responsibility is to identify what we can prove, what remains uncertain, and how to systematically close those gaps.

Do not confuse implementation activity with product progress.

---

# 1. Understand the complete AutoSteer vision

AutoSteer is a language-agnostic, agent-independent engineering intelligence and active steering system.

Its purpose is to help AI coding agents operate correctly and efficiently in complex, mature, interconnected software systems.

Modern coding agents can read, reason about, and modify code. However, they are not guaranteed to discover every relevant dependency, architectural constraint, established abstraction, historical decision, environment-specific requirement, or downstream consequence of a change.

These limitations become increasingly significant in systems containing:

- Hundreds of thousands or millions of lines of code.
- Multiple interconnected repositories and services.
- Several programming languages and frameworks.
- Shared contracts, APIs, event streams, databases, and infrastructure.
- Business and technical workflows spanning system boundaries.
- Organization-specific engineering conventions.
- Historical incidents, exceptions, and architectural decisions.
- Multiple developers and coding agents working across sessions.
- Long-running tasks involving context compaction, interruptions, and handoffs.

The objective is not to summarize a repository or replace the coding model's reasoning.

**AutoSteer should maintain useful engineering intelligence about a system, identify which parts matter to the current task, expose dependencies the agent did not know to search for, guide implementation decisions, preserve context across execution, independently verify outcomes, and improve its knowledge through evidence-backed learning.**

The complete loop is:

**Project knowledge → Task understanding → Relevant architectural context → Agent decision → Observed implementation → Independent verification → Updated project and session knowledge.**

This loop is the product.

Everything we build must demonstrate how it contributes to that loop.

---

# 2. Preserve the four functional graphs and one decision engine

Our current architectural direction uses four interconnected functional graphs.

Treat these as responsibilities we have agreed to explore and validate, not as permission to build four unnecessarily independent graph databases.

## A. Platform graph — What exists and how it connects

The platform graph represents the engineering system itself.

It should capture, where technically discoverable:

- Repositories, modules, packages, files, symbols, and logical components.
- Static and dynamic code dependencies.
- Function calls, references, inheritance, interfaces, and implementations.
- Service boundaries and ownership relationships.
- Internal and external APIs.
- Event publishers, subscribers, brokers, and message contracts.
- Database schemas, tables, readers, and writers.
- Configuration, feature flags, and environment dependencies.
- Existing abstractions and reusable implementations.
- Tests and the components or behaviors they cover.
- Cross-repository and cross-language relationships.
- Architectural boundaries and the consequences of crossing them.

It must distinguish between proven, inferred, possible, unresolved, and unindexed relationships.

It must support incremental changes, working-tree state, source provenance, graph coverage, and revision-aware queries.

The key question:

**Can this graph identify important architectural relationships the agent would otherwise overlook, including relationships outside the current repository?**

## B. Process graph — How the system and engineering workflows behave

The process graph represents expected and observed workflows.

It should model both:

1. Technical and business processes within the software.
2. Engineering workflows and verification requirements.

Examples include request processing, asynchronous event handling, authentication workflows, CI pipelines, deployment gates, schema migrations, test obligations, and required development sequences.

It should support a distinction between expected process templates and observed execution instances.

It must explain which operations depend on others, which conditions must hold, which gates apply, and what remains unfinished.

The key question:

**When an agent changes a component, can AutoSteer identify which technical or engineering processes may be affected and what must be verified?**

## C. Knowledge DAG — Why the system is designed this way

The knowledge DAG represents engineering decisions and their justification.

It should capture:

- Architectural decisions and rejected alternatives.
- Project conventions and standards.
- Known limitations and environmental constraints.
- Historical incidents and their root causes.
- Validated lessons from previous sessions.
- Explicit exceptions and their scope.
- Evidence supporting or contradicting engineering knowledge.
- Supersession, invalidation, and changes in applicability.

Knowledge must not become authoritative simply because an agent generated it or repeated it.

Determine how AutoSteer can accumulate useful engineering knowledge without accumulating incorrect assumptions.

The key question:

**Can AutoSteer preserve the engineering knowledge that ordinarily disappears into individual developers' experience, review discussions, and previous agent sessions?**

## D. Session graph — What the agent is trying to do and what actually happens

The session graph is a continuously updated model of the active engineering task.

It should track:

- Original user intent.
- Explicit requirements and inferred requirements, separately.
- Relevant project context.
- The agent's declared plan.
- Decisions made and their justification.
- Files and components investigated.
- Changes made and their observed outcomes.
- Guidance delivered by AutoSteer.
- Verification obligations and unresolved issues.
- Newly discovered constraints.
- Progress toward the original objective.
- Context compactions, interruptions, resumptions, and agent handoffs.

The system should update from every observable relevant event without assuming that every internal model decision is observable.

**The session graph must not be merely a history of tool calls. It must preserve the engineering intent and state necessary to continue the task correctly.**

A central requirement is that the system remains synchronized throughout the session, while delivering context selectively rather than injecting unnecessary text after every action.

## E. One task-aware decision engine

The decision engine connects the graphs.

Its core functional stages are:

1. **Identify Scope:** Interpret the task and identify relevant architectural entry points.
2. **Traverse:** Discover associated components, dependencies, processes, constraints, and knowledge.
3. **Rank:** Prioritize the information most useful to the current engineering decision.
4. **Check Conflicts:** Evaluate plans and observed changes against applicable constraints.
5. **Select Guidance:** Deliver the right evidence, at the right time, within a controlled context budget.

The decision engine must remain language-agnostic and agent-independent.

Its output should be actionable engineering intelligence, not generic repository summaries.

---

# 3. Preserve the distinction between G, A, and D

This is a foundational requirement.

**G — What AutoSteer's project intelligence knows.**

**A — What information the coding agent has been exposed to, or has demonstrably acted upon.**

**D — What the coding agent actually does, as independently observed.**

Never conflate these.

A graph containing a relationship does not mean the agent knows it.

An injected instruction does not mean the agent understood it.

An agent opening a file does not prove it used the relevant information.

An agent following a recommendation does not prove that recommendation improved correctness.

A passing test does not establish architectural compliance.

Design the system so these distinctions survive into its records, learning policies, measurements, and user-facing reports.

---

# 4. Establish the actual state of Rashomon

Before recommending architectural changes, inspect the latest repository state.

Review the latest branch, commits, relevant worktrees, implementation files, tests, design documents, and recorded experiments.

Do not rely exclusively on README statements or historical assessments.

Prior research documents include:

- The November 3 end-to-end architecture assessment.
- The November 3 architecture and foundation contracts.
- The November 4 engineering-intelligence direction.
- The November 4 trace-and-contracts design.
- The revised contracts and implementation records.
- The most recent pipeline evidence and experimental results.

Locate and read the available documents directly.

Distinguish:

- Implemented and exercised in production paths.
- Implemented but only tested in isolation.
- Integrated but not validated through an actual coding agent.
- Experimental.
- Designed but unimplemented.
- Superseded or abandoned.

Preserve existing engineering strengths where appropriate, especially independent recording, consent, provenance, gate verification, deterministic evidence handling, and bounded steering.

Do not replace working components merely because a different library exists.

## Important historical findings to verify

Our recent experiments have suggested that:

- Cross-repository architectural context can change which files an agent investigates.
- A pure-graph experiment reported four of five steered runs opening a relevant sibling-repository file versus zero of five baseline runs.
- A separate experiment suggested that the full graph pipeline outperformed a lessons-only approach.
- Steered runs sometimes consumed more tokens and cost more because they performed additional relevant work.
- An unseeded task exposed shortcomings in ranking and selection.
- A lesson-labeling defect incorrectly interpreted compliance with a rule that required leaving a component unchanged.
- Some architectural capabilities remain limited to one language or experimental fixtures.
- Several important components are not yet demonstrated as a complete, reliable end-to-end product.

These are preliminary historical observations, not established performance claims.

Verify what remains true in the current code.

Identify which experiments need repetition, stronger controls, or real-world validation.

---

# 5. Research the best existing engineering implementations

I want a serious investigation into **which parts of AutoSteer's architecture are already solved well enough elsewhere that we should adapt or reuse them**.

Do not reinvent established infrastructure unnecessarily.

However, do not treat the existence of an implementation as proof that it fits our requirements.

Evaluate technical compatibility, accuracy, maintenance cost, licenses, operational overhead, and potential impact on AutoSteer's differentiation.

Study the following areas.

## A. Code intelligence and semantic indexing

Investigate:

- Sourcegraph SCIP and maintained SCIP indexers.
- Tree-sitter.
- Graphify.
- Joern.
- Other mature code-property-graph or language-server approaches where relevant.

Determine how they represent:

- Symbol identities.
- Cross-language relationships.
- Calls, references, and implementations.
- Cross-repository contracts.
- Dynamic or unresolved dependencies.
- Incremental updates.
- Uncommitted changes.
- Provenance and confidence.
- Graph coverage and incomplete information.

Specifically challenge whether we need to continue building custom language extractors.

Can we use mature semantic indexers for structure while maintaining separate AutoSteer-specific extractors for contracts, SQL relationships, runtime evidence, configuration, and historical coupling?

Evaluate whether SCIP should become an ingestion format rather than a replacement for our existing graph model.

Preserve the semantic accuracy of pgraph for Go.

Do not make the core dependent on Go-specific concepts.

## B. Task understanding and intelligent retrieval

Investigate:

- Aider's repository map and personalized PageRank.
- Graphify's query and retrieval approaches.
- Available open-source hybrid search and graph-retrieval techniques.
- Lexical, structural, semantic, and graph-based relevance ranking.
- Query decomposition and task-oriented retrieval.
- Ranking calibrated by graph coverage, evidence quality, recency, and architectural importance.

I particularly want to understand how an ambiguous engineering task can lead AutoSteer to a dependency the agent would not independently identify.

Compare alternative ranking mechanisms against our existing Scope → Traverse → Rank → Select pipeline.

Aider's `repomap.py` should be an early concrete investigation, but it must not become the limit of this research.

Ask whether personalized PageRank is appropriate for our heterogeneous graph of code relationships, contracts, historical knowledge, boundaries, and process obligations.

Study how to represent and retrieve cross-repository dependencies that have no direct source-code import relationship.

Evaluate retrieval quality, latency, uncertainty, and context-token cost.

## C. Architectural conventions and policy enforcement

Investigate:

- Semgrep.
- dependency-cruiser.
- Open Policy Agent.
- Relevant architecture-conformance and static-analysis systems.

Determine how to represent and enforce:

- Forbidden dependency directions.
- Approved abstraction usage.
- Service and data ownership boundaries.
- Security and trust boundaries.
- Environment restrictions.
- Required verification gates.
- Explicit policy exceptions.
- Versioned organizational conventions.

Distinguish objectively checkable constraints from engineering judgments that cannot be reliably expressed as deterministic predicates.

Study how enforcement can be useful without becoming excessively noisy or overconfident.

## D. Persistent knowledge and decision history

Investigate:

- Graphiti and other temporal knowledge-graph techniques.
- Truth-maintenance systems.
- Event sourcing.
- Decision-record and provenance models.
- Relevant persistent-memory systems.

Determine how AutoSteer should support:

- Fact creation and validation.
- Evidence-linked decisions.
- Historical changes.
- Supersession and invalidation.
- Repeated engineering lessons.
- Cross-session and cross-team knowledge reuse.
- Trust and authorization.
- Prevention of false or stale knowledge propagation.

Evaluate whether existing temporal knowledge techniques can be implemented within our local-first architecture without requiring heavyweight graph services or unnecessary model calls.

## E. Session continuity and agent state

Investigate:

- OpenHands event histories and condensation.
- LangGraph state and checkpointing.
- Agent memory architectures.
- Event-driven state reconstruction.
- Agent handoff protocols and available harness hooks.

Determine what can be reused conceptually or directly to support our session graph.

The core goal is not generic conversational memory.

It is **persistent engineering intent, decisions, constraints, actions, and unresolved obligations synchronized with the work**.

Evaluate real hook limitations and avoid assuming access to a model's private reasoning.

## F. Runtime steering and verification

Investigate mature approaches for:

- Plan-time guidance.
- Just-in-time context delivery.
- Post-edit verification.
- Architectural compliance checks.
- Tool-call observation.
- Task-outcome evaluation.
- Agent trajectory measurement.
- Evidence-backed learning from outcomes.

Consider existing evaluation systems, including promptfoo and relevant coding-agent benchmarks.

Determine which techniques can strengthen Rashomon without weakening its independence from the coding agent.

## G. Multi-agent interoperability and developer experience

Investigate agent integration approaches for:

- Claude Code.
- Codex.
- Cursor.
- Other widely used coding agents where documented interfaces exist.

Determine the common abstraction and the adapter-specific limitations.

Study REA's open-source installation and multi-agent setup implementation if useful for developer experience.

REA is not required for studying open-source code. Prefer direct source inspection. Use reverse-engineering techniques only where genuinely necessary, authorized, and legally appropriate.

Our objective is to provide consistent intelligence across coding agents without depending on undocumented behavior or assuming all agents expose equivalent hooks.

---

# 6. Evaluate complete functional coverage

Do not restrict the investigation to the capabilities demonstrated by our existing fixtures.

Evaluate the intended AutoSteer product against realistic engineering situations.

At minimum, cover:

**Scenario 1 — Unknown architectural dependency**

An agent receives a vague task and must identify a downstream component or dependency the prompt never mentioned.

**Scenario 2 — Existing functionality**

An agent is about to implement something that already exists elsewhere in the repository.

**Scenario 3 — Cross-repository change**

A change in one service affects consumers or contracts in another repository, possibly written in a different language.

**Scenario 4 — Architectural convention conflict**

An implementation works and passes tests but violates a documented dependency boundary or an approved design decision.

**Scenario 5 — Business process impact**

A change affects a workflow spanning synchronous calls, asynchronous events, database state, and background processing.

**Scenario 6 — Long-running session**

A task undergoes multiple compactions, interruptions, and resumptions. Important requirements and decisions must survive.

**Scenario 7 — Multiple agents**

One agent begins a task and another continues it, without losing relevant architectural context or verification obligations.

**Scenario 8 — Historical engineering knowledge**

A previous incident or rejected architectural approach should influence a new implementation.

**Scenario 9 — Evolving repository**

The agent makes multiple uncommitted edits, and AutoSteer must reason about the current working tree rather than only the latest committed snapshot.

**Scenario 10 — Incomplete or conflicting knowledge**

A graph relationship is uncertain, stale, dynamically resolved, or contradicted by newer evidence.

**Scenario 11 — Enterprise-scale system**

The task spans a large monorepo or several interconnected repositories with incomplete indexing, heterogeneous languages, and substantial architectural complexity.

**Scenario 12 — Cost and efficiency**

Determine when proactive context reduces unnecessary exploration and when it appropriately causes the agent to perform more work to reach a correct outcome.

For each scenario, explain:

- What must be understood.
- What sources provide the necessary evidence.
- Which graph responsibilities participate.
- How the decision engine selects relevant information.
- When and how guidance reaches the agent.
- How AutoSteer knows whether the agent acted on it.
- What can be independently verified.
- What remains unknowable.
- Which current Rashomon components support the scenario.
- Which capabilities are genuinely missing.

Do not mark a scenario supported because its required graph schema exists.

It is supported only when the relevant end-to-end behavior has been demonstrated.

---

# 7. Challenge the foundational architecture

Identify architectural decisions that may become expensive to reverse.

Pay particular attention to:

1. **Identity:** Stable, scoped identities for repositories, modules, symbols, logical components, and contracts across revisions and languages.
2. **Graph representation:** Whether four functional projections need independent storage or can share a coherent evidence-backed model.
3. **Knowledge provenance:** How assertions, observations, inferences, approved rules, and measured outcomes differ.
4. **Graph freshness:** Consistent reasoning over committed snapshots, uncommitted edits, and external repository changes.
5. **Cross-repository joins:** How contract equivalence is demonstrated rather than assumed from similar names.
6. **Task interpretation:** How natural-language intent is converted into relevant architectural scope.
7. **Retrieval:** How ranking handles unknown dependencies, incomplete evidence, and different graph relationship types.
8. **Session synchronization:** How the system reconstructs reliable task state from observable agent activity.
9. **Learning:** How it avoids treating correlation, test success, or superficial action matching as proof that guidance helped.
10. **Adapter independence:** How different agent capabilities are represented without contaminating core contracts.
11. **Performance:** How indexing, retrieval, and steering remain fast in large repositories.
12. **Security and governance:** How trusted declarations, untrusted repository content, team permissions, and information disclosure are handled.

For every decision, determine whether we should preserve, revise, replace, or defer the current approach.

Do not assume the current architecture is correct simply because it is well documented or heavily tested.

Equally, do not propose replacing it without evidence of a meaningful advantage.

---

# 8. Make engineering intelligence the center of the design

This is the most important section.

I do not want AutoSteer to become a large verification framework with a small context-retrieval feature attached.

The core product must be **the intelligence that improves the agent's engineering decisions**.

I want you to investigate how the system can:

- Understand what the developer is trying to accomplish.
- Identify relevant architecture before implementation.
- Discover important but unnamed dependencies.
- Surface existing abstractions instead of encouraging duplication.
- Explain applicable constraints and their rationale.
- Help an agent formulate and validate an appropriate implementation strategy.
- Identify nonlocal consequences of local changes.
- Detect architectural conflicts during implementation.
- Preserve decisions and requirements as work evolves.
- Verify that the final implementation is consistent with the original goal.
- Turn useful discoveries into reusable knowledge.

Distinguish repository search, graph retrieval, task interpretation, engineering reasoning, runtime intervention, and independent verification.

They are related but not interchangeable.

Determine exactly where model-based semantic reasoning adds value and where deterministic analysis is preferable.

Do not put expensive, unpredictable model inference in latency-critical hooks without compelling evidence.

However, do not reject model-assisted knowledge acquisition or task reasoning merely to preserve a fully deterministic architecture.

The proper objective is **the most reliable and useful system**, with well-defined trust boundaries and measurable costs.

---

# 9. Research should lead to engineering decisions

I want comprehensive research, but not an endless collection of project summaries.

For every external implementation you recommend, establish:

- The actual problem it solves.
- The algorithm or architecture it uses.
- Source-level evidence.
- Its relevant strengths and limitations.
- Compatibility with Rashomon.
- What it would replace or extend.
- Dependencies, license, and operational implications.
- The smallest experiment that could validate its value.
- The measurable result that would justify adoption.

Categorize recommendations as:

**Adopt directly, adapt selectively, implement independently, investigate further, or reject.**

Do not combine five frameworks simply because all five solve relevant problems.

The result must be a cohesive AutoSteer architecture, not a collection of incompatible vendor concepts.

Prioritize research according to engineering dependency and expected product impact.

It is acceptable to investigate the complete ecosystem while recommending that only one or two experiments be implemented immediately.

Comprehensive understanding and disciplined execution are not contradictory.

---

# 10. Establish rigorous quality and readiness standards

We should not call the product alpha-ready simply because installation works, the tests pass, or one demonstration succeeds.

Define readiness gates separately for:

## A. Structural correctness

Graph identity, relationships, provenance, uncertainty, indexing coverage, working-tree consistency, and cross-language behavior.

## B. Task intelligence

Ability to resolve ambiguous tasks, identify correct architectural neighborhoods, find unnamed dependencies, and avoid false-positive guidance.

## C. Behavioral improvement

Evidence that guidance causes better implementation decisions compared with equivalent unaided agent runs.

## D. Execution continuity

Correct preservation of task state, requirements, decisions, obligations, and context across long sessions and supported agent transitions.

## E. Independent verification

Accurate claims about tests, gates, architectural constraints, functional outcomes, and unresolved work.

## F. Learning reliability

Evidence-backed promotion, invalidation, recurrence prevention, and resistance to accumulating incorrect knowledge.

## G. Language and repository coverage

Useful behavior across supported languages, repositories, build systems, and integration patterns.

## H. Performance and cost

Indexing overhead, retrieval latency, context tokens, tool calls, wall-clock time, and cost per correct engineering outcome.

## I. Security and operational reliability

Consent, access boundaries, untrusted content, local data handling, failure recovery, and reproducible deployment.

## J. Developer experience

Installation, onboarding, comprehensible guidance, explainable reports, troubleshooting, and graceful degradation.

For each category, define objective acceptance criteria and representative test environments.

Distinguish a successful synthetic fixture, a successful real-repository experiment, a reliable developer preview, an alpha, a beta, and a production-grade offering.

Do not manufacture performance claims or assume that a small number of agent runs establishes statistical confidence.

---

# 11. Evaluate the competitive and enterprise product opportunity

Consider the complete competitive landscape, including:

- Native coding-agent context and memory systems.
- Codebase indexing and retrieval providers.
- Architecture and dependency analysis tools.
- Code review and engineering-governance platforms.
- Agent orchestration and runtime frameworks.
- Persistent memory and knowledge-graph systems.

Explain what customers can already achieve by combining existing solutions.

Then identify what AutoSteer would provide beyond those combinations.

Challenge whether our differentiation is truly defensible.

Evaluate the strongest potential value propositions:

- Architecture-aware agent development.
- Cross-repository engineering intelligence.
- Persistent project and organizational knowledge.
- Active prevention of architectural mistakes.
- Reliable context continuity.
- Independent implementation verification.
- Lower rework and cost per correct implementation.
- Consistent behavior across different coding agents.

Distinguish what we have demonstrated from what we hope to deliver.

Also consider open-source adoption and enterprise adoption separately.

Do not confuse GitHub stars with product quality, and do not assume enterprise demand simply because the problem sounds compelling.

Identify realistic paths from individual developer value to team and organizational adoption.

---

# 12. Produce a complete but prioritized engineering roadmap

I want an execution strategy that respects the entire product vision without trying to implement everything simultaneously.

Separate the work into:

**Foundation:** Architectural contracts that need to be correct before multiple subsystems depend on them.

**Core intelligence:** Acquisition, task understanding, graph traversal, relevance ranking, and evidence-backed guidance.

**Execution integration:** Active steering, working-tree awareness, agent interfaces, and session synchronization.

**Trust:** Verification, provenance, learning validity, and failure handling.

**Scale:** Multi-language support, cross-repository indexing, performance, and enterprise deployment.

**Developer experience:** Installation, reporting, inspectability, documentation, and usability.

For each proposed workstream, specify:

- Current implementation state.
- Missing capabilities.
- Exact relevant source packages.
- External implementations worth adapting.
- Architectural dependencies.
- Required tests.
- Risks and tradeoffs.
- Observable completion criteria.
- What can be deferred without compromising future design.

Explicitly distinguish **foundational contracts that must be decided now** from features that can evolve safely later.

Do not impose an artificial development schedule.

Do not use an upcoming release date as a reason to lower correctness standards.

---

# 13. Identify the strongest next experiments

After the complete investigation, select the experiments that produce the greatest reduction in uncertainty about AutoSteer's product value.

The experiments should test different failure classes, not merely repeat the same successful demonstration.

At minimum, consider:

1. Whether a better relevance algorithm can discover an unknown dependency without historical knowledge.
2. Whether mature semantic indexers can significantly improve language coverage without sacrificing accuracy.
3. Whether project knowledge can reliably alter implementation planning in a mature repository.
4. Whether the session graph preserves requirements and decisions over long-running execution.
5. Whether independent verification detects architecturally incorrect implementations that ordinary tests miss.
6. Whether evidence-backed learning improves future sessions without reinforcing false conclusions.

Use existing experiments and fixtures where appropriate, but challenge whether they are representative.

Include negative controls, independent evaluation, baselines, and explicit kill criteria.

Evaluate correctness and cost together.

A steered agent performing more work may still be more efficient if it avoids expensive downstream failures. Conversely, merely opening additional files does not prove better engineering.

---

# 14. Required deliverables

Produce the following, grounded in actual repository inspection and external source research.

### Deliverable 1 — Product definition

A precise explanation of the complete AutoSteer product, its essential behaviors, user value, and differentiating capabilities.

### Deliverable 2 — Current implementation assessment

A source-referenced inventory of the current Rashomon architecture, identifying implemented, integrated, experimental, missing, and superseded functionality.

### Deliverable 3 — External technology research

A technically rigorous evaluation of the strongest existing implementations that could accelerate development, with direct source references and an explicit build-versus-adapt recommendation.

### Deliverable 4 — Target system architecture

The end-to-end functional architecture connecting all four graphs, the decision engine, agent runtime, verification, and learning.

Include data flows, update triggers, trust boundaries, and interfaces.

### Deliverable 5 — Foundational architectural decisions

The decisions that must be correct now to avoid expensive future redesigns, including alternatives and explicit tradeoffs.

### Deliverable 6 — Functional execution traces

Detailed traces of representative tasks, including ambiguous intent, cross-repository dependencies, architectural conflicts, compaction, and learning.

Distinguish expected behavior from functionality already demonstrated.

### Deliverable 7 — Quality and readiness framework

A rigorous, evidence-driven definition of what must be demonstrated before AutoSteer deserves to be called a working developer preview, alpha, beta, or production-ready product.

### Deliverable 8 — Prioritized engineering roadmap

An implementation sequence that moves from the current repository toward the complete product while preserving architectural integrity.

### Deliverable 9 — Immediate experiments

The highest-value experiments to perform next, with baselines, acceptance criteria, failure interpretations, and kill conditions.

### Deliverable 10 — Critical dissent

Identify the strongest reasons the vision, architecture, or product strategy could fail.

Challenge my assumptions and your own.

Explain what evidence would cause you to fundamentally change the architecture or product direction.

---

# 15. Execution instructions

Do not begin by implementing another subsystem.

First establish the actual state of the repository and the relevant external technologies.

Read real source code, not just marketing descriptions or README summaries.

Use direct source inspection for open-source projects. Do not install REA or use binary reverse engineering when source code is available.

Do not launch an unrestricted swarm of agents or perform expensive research without a clear division of questions and evidence.

Do not silently rewrite or discard the existing architecture.

Do not modify production code during this investigation. Tests and isolated prototypes may be proposed, but implementation should follow the architecture and experiment decisions.

Do not produce another abstract vision document with no executable implications.

Every major recommendation must identify how it changes actual AutoSteer behavior.

Every proposed technology must solve a defined problem.

Every proposed new subsystem must have an identified consumer.

Every claimed improvement must have a verification mechanism.

Every major uncertainty must remain visible.

## The questions I ultimately want answered

**1. Does the complete AutoSteer vision make technical and product sense?**

**2. What parts of that vision are already solved by mature open-source technologies?**

**3. What parts should AutoSteer own because they constitute its differentiated intelligence?**

**4. Are our four functional graphs and decision engine the correct foundations, or are important responsibilities missing?**

**5. What foundational design mistakes exist in Rashomon today?**

**6. How can AutoSteer reliably identify architectural knowledge an agent would otherwise miss?**

**7. How can it keep that knowledge synchronized with the project, the current task, and the agent's observed behavior?**

**8. How do we verify that the intelligence actually improves engineering outcomes?**

**9. What would make the system genuinely useful in large, multi-repository, enterprise environments?**

**10. What should we build next, in what order, and what evidence would justify each investment?**

### Final principle

**We are building AutoSteer to improve how AI agents engineer software, not simply to observe them, store graphs, or provide more context.**

The platform graph, process graph, knowledge DAG, session graph, decision engine, steering runtime, verification system, and learning loop must function as one coherent product.

Do not optimize for the fastest apparent MVP.

Optimize for a technically sound architecture, a reliable path to demonstrated engineering value, and a foundation capable of supporting the complete product without repeated structural redesigns.

I want to understand what a genuinely excellent AutoSteer would require—and how much of that engineering we can accelerate by intelligently building on solutions that already exist.

**Be ambitious in the investigation, skeptical in the engineering, precise in the evidence, and disciplined in the implementation recommendations.**
