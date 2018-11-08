# Alert2amqp

This microservice receives alerts from the Prometheus alertmanager and puts them
in an AMQP queue.

## Expectations

This microservice is expecting groups of *1* alert. It means that it is
alertmanager job to group the alerts one by one.

## Out of scope

Except the 1-alert condition mentioned above, we do not do any other change or
check on the alerts. The alerts are simply posted to the queue.

## AMQP

Only the alert is posted, not the alert group.

Expiration of the message is currently set to 365 days.

## known issues

The user name and password come from an environment variable, they should come
from a file.

## License

Apache-2.0
