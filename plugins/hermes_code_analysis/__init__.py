"""Hermes Code Analysis plugin."""
import logging

logger = logging.getLogger(__name__)
try:
    from tools.registry import registry
    from hermes_code_analysis.tools import CODE_TOOLS
    
    for tool_name, tool_def in CODE_TOOLS.items():
        registry.register(
            name=tool_name,
            toolset="code_analysis",
            schema={"type": "object", "properties": {"path": {"type": "string"}}},
            handler=tool_def["handler"],
            check_fn=lambda: True,
            emoji="🔍",
            description=tool_def.get("description", ""),
        )
    logger.info("hermes-code-analysis plugin loaded: %d tools registered", len(CODE_TOOLS))
except ImportError as e:
    logger.debug("hermes-code-analysis plugin not loaded: %s", e)
