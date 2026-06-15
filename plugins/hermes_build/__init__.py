"""Hermes Build plugin — build system tools."""
import logging

logger = logging.getLogger(__name__)
try:
    from tools.registry import registry
    from hermes_build.tools import BUILD_TOOLS
    
    for tool_name, tool_def in BUILD_TOOLS.items():
        registry.register(
            name=tool_name,
            toolset="build",
            schema={"type": "object", "properties": {"cwd": {"type": "string"}}},
            handler=tool_def["handler"],
            check_fn=lambda: True,
            emoji="🏗️",
            description=tool_def.get("description", ""),
        )
    logger.info("hermes-build plugin loaded: %d tools registered", len(BUILD_TOOLS))
except ImportError as e:
    logger.debug("hermes-build plugin not loaded: %s", e)
