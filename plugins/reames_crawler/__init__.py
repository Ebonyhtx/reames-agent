"""Hermes Crawler plugin — web scraping tools."""
import logging

logger = logging.getLogger(__name__)
try:
    from tools.registry import registry
    from reames_crawler.tools import CRAWLER_TOOLS
    
    for tool_name, tool_def in CRAWLER_TOOLS.items():
        registry.register(
            name=tool_name,
            toolset="crawler",
            schema=tool_def["schema"],
            handler=tool_def["handler"],
            check_fn=lambda: True,
            emoji="🕷️",
            description=tool_def.get("description", ""),
        )
    logger.info("hermes-crawler plugin loaded: %d tools registered", len(CRAWLER_TOOLS))
except ImportError as e:
    logger.debug("hermes-crawler plugin not loaded: %s", e)
