class NotFoundError(Exception):
    """The requested resource is absent from the caller's shop and batch."""


class ConflictError(Exception):
    """The requested transition conflicts with persisted state."""
