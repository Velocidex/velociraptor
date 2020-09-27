import "./client-summary.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { Link } from  "react-router-dom";

import VeloClientStatusIcon from "./client-status.js";

export default class VeloClientSummary extends Component {
    static propTypes = {
        client: PropTypes.object,
    }

    timeDifference(last_seen_at) {
        var currentTimeMs = new Date().getTime();
        var inputTimeMs = last_seen_at / 1000;

        if (inputTimeMs < 1e-6) {
            return '<invalid time value>';
        }

        var differenceSec = Math.abs(
            Math.round((currentTimeMs - inputTimeMs) / 1000));

        var measureUnit;
        var measureValue;
        if (differenceSec < 15) {
            return "connected";
        } else if (differenceSec < 60) {
            measureUnit = 'seconds';
            measureValue = differenceSec;
        } else if (differenceSec < 60 * 60) {
            measureUnit = 'minutes';
            measureValue = Math.floor(differenceSec / 60);
        } else if (differenceSec < 60 * 60 * 24) {
            measureUnit = 'hours';
            measureValue = Math.floor(differenceSec / (60 * 60));
        } else {
            measureUnit = 'days';
            measureValue = Math.floor(differenceSec / (60 * 60 * 24));
        }

        if (currentTimeMs >= inputTimeMs) {
            return measureValue + ' ' + measureUnit + ' ago';
        };

        return "";
    };

    render() {
        if (!this.props.client ||
            !this.props.client.client_id) {
            return <></>;
        }

        var fqdn = this.props.client.client_id;
        if (this.props.client.os_info && this.props.client.os_info.fqdn) {
            fqdn = this.props.client.os_info.fqdn;
        }

        return (
            <div className="client-summary client-status toolbar-buttons">
              <Link className="nav-link client-name"
                to={"/host/"+this.props.client.client_id}>
                { fqdn }
              </Link>
              <span className="centered">
                <VeloClientStatusIcon client={this.props.client}/>
              </span>
              { this.timeDifference(this.props.client.last_seen_at) }
            </div>

        );
    };
}
