# RES-5: PostPayload.from reads prompt_id, agent_id and permission_mode for the
# steer step (card 2.3), and a wrongly typed one fails the whole intake, as a
# struct with the field would. Dropping the agent_id read lets
# {"agent_id":7777} through while the oracle's struct refuses it, and leaves
# a well-typed agent_id unread (FUZZ-RES5-AGENT1, R50).
m("RES-5 the post intake stops reading agent_id", "internal/hook/post.go",
  "hookio.Into(hookio.PromptID, &pl.PromptID), hookio.Into(hookio.AgentID, &pl.AgentID),",
  "hookio.Into(hookio.PromptID, &pl.PromptID),",
  "TestRES5_|FuzzRES5_IntakeMatchesTheStructDecode")
