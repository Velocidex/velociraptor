import "./client-status.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

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
        let now = date.getTime();

        if ((now - item.last_seen_at/1000) < 60 * 15) {
            return  <img className="icon-small" src={online}/>;
        }
        if ((now - item.last_seen_at/1000) < 60 * 60 * 24) {
            return  <img className="icon-small" src={online1d} />;
        }
        return  <img className="icon-small" src={offline} />;
    }
}
