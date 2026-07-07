"""Reames Git plugin — development workflow tools."""
import logging

logger = logging.getLogger(__name__)
try:
    from tools.registry import registry
    from reames_git.tools import GIT_TOOLS
    
    for tool_name, tool_def in GIT_TOOLS.items():
        registry.register(
            name=tool_name,
            toolset="git",
            schema=tool_def["schema"],
            handler=tool_def["handler"],
            check_fn=lambda: True,
            emoji="🔀",
            description=tool_def.get("description", ""),
        )
    logger.info("reames-git plugin loaded: %d tools registered", len(GIT_TOOLS))
except ImportError as e:
    logger.debug("reames-git plugin not loaded: %s", e)
