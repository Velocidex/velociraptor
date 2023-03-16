import "./client-status.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import online from './img/online.png';
import online1d from './img/online-1d.png';
import offline from './img/offline.png';


export default class VeloClientStatusIcon extends Component {
    static propTypes = {
        client: PropTypes.object,
    }

    render() {
        let item = this.props.client;
        let date = new Date();
        let now_ms = date.getTime();
        let last_seen_ms = item.last_seen_at/1000;
        if ((now_ms - last_seen_ms) < (60 * 15 * 1000)) {
            return  <img className="icon-small" src={api.src_of(online)} alt="online" />;
        }
        if ((now_ms - last_seen_ms) < (60 * 60 * 24 * 1000)) {
            return  <img className="icon-small" src={api.src_of(online1d)} alt="online1d" />;
        }
        return  <img className="icon-small" src={api.src_of(offline)} alt="offline" />;
    }
}
