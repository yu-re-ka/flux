// `sendEvent` sends a trigger event to PagerDuty
sendEvent = (routingKey,
    links,
    client,
    clientURL,
    dedupKey,
    customDetails,
    class,
    group,
    eventAction,
    severity,
    source,
    summary,
    timestamp
) => {
    data = {
        "payload": {
            "summary": summary,
            "timestamp": timestamp,
            "source": source,
            "severity": severity,
            "group": group,
            "class": class,
            "custom_details": customDetails
        },
        "routing_key": routingKey,
        "dedup_key": dedupKey,
        "links": links,
        "event_action": eventAction,
        "client": client,
        "client_url": clientURL
    }

    header = {
        "Authorization": token,
        "Content Type": application/json"
    }
    enc = json.encode(data)
    return http.post(header: header, url: url, data: enc)
}


endpoint = (url) =>
    (mapFn) =>
        (tables=<-) => tables
            |> map(fn: (r) => {
                obj = mapFn(r)

                resp = sendEvent(
                    routingKey: obj.routingKey,
                    links: obj.links,
                    client: obj.client,
                    clientURL: obj.clientURL,
                    dedupKey: obj.dedupKey,
                    customDetails: obj.customDetails,
                    class: obj.class,
                    group: obj.group,
                    eventAction: obj.eventAction,
                    severity: obj.severity,
                    source: obj.source,
                    summary: obj.summary,
                    timestamp: obj.timestamp
                )
                return {r with status: resp.status}
            })
             