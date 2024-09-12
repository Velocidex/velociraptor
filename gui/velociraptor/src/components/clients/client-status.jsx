import "./client-status.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

// In ms
const Duration15Min = 60 * 15 * 1000;
const Duration24Hours = 60 * 60 * 24 * 1000;

export default class VeloClientStatusIcon extends Component {
    static propTypes = {
        client: PropTypes.object,
    }

    render() {
        let item = this.props.client;
        let date = new Date();
        let now_ms = date.getTime();
        let last_seen_ms = item.last_seen_at/1000;
        if ((now_ms - last_seen_ms) < Duration15Min) {
            return  <div className="online-btn" alt="online" />;
        }
        if ((now_ms - last_seen_ms) < Duration24Hours) {
            return  <FontAwesomeIcon icon="fa-solid fa-triangle-exclamation"
                                 className="fa-fade fa-offline-btn" alt="online1d" />;
        }
        return <FontAwesomeIcon icon="fa-solid fa-triangle-exclamation"
                                className="fa-offline-btn " alt="online1d" />;
    }
}
