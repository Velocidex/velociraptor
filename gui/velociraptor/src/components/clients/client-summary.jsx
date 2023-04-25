import "./client-summary.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';

import { Link } from  "react-router-dom";

import VeloClientStatusIcon from "./client-status.jsx";

const POLL_TIME = 5000;

export default class VeloClientSummary extends Component {
    static propTypes = {
        client: PropTypes.object,
        setClient: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.getClientInfo, POLL_TIME);
        this.getClientInfo();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let old_client_id = prevProps.client && prevProps.client.client_id;
        let new_client_id = this.props.client && this.props.client.client_id;
        if (old_client_id !== new_client_id) {
            clearInterval(this.interval);
            this.interval = setInterval(this.getClientInfo, POLL_TIME);
            this.getClientInfo();
        }
    }

    getClientInfo = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        let params = {update_mru: true};
        let client_id = this.props.client && this.props.client.client_id;
        if (client_id) {
            api.get("v1/GetClient/" + client_id, params,
                    this.source.token).then(
                response=>{
                    if (response.cancel) return;
                    if (!response.data || response.data.client_id !== client_id) {
                        return;
                    }
                    this.props.setClient(response.data);
                }).catch(err=>{
                    // The client is not valid - navigate away from
                    // it.
                    // this.props.setClient({});
                    return false;
                });
        }
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
            return T("Connected");
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
            return T("time_ago", measureValue, measureUnit);
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
              <span className="centered client-status">
                <VeloClientStatusIcon client={this.props.client}/>
              </span>
              { this.timeDifference(this.props.client.last_seen_at) }
            </div>

        );
    };
}
