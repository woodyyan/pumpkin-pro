class DataSourceError(RuntimeError):
    """Base error for data source gateway failures."""


class UnsupportedCapabilityError(DataSourceError):
    pass


class UnsupportedMarketError(DataSourceError):
    pass


class EmptyResponseError(DataSourceError):
    pass


class ValidationError(DataSourceError):
    pass


class TradeDateMismatchError(ValidationError):
    pass
