import "./time.css";
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';
import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import humanizeDuration from "humanize-duration";

const renderToolTip = (props, ts) => {
    let now = new Date().getTime();
    let difference = (now-ts.getTime());
    if (difference < 0) {
        return <Tooltip {...props}>
                 In {humanizeDuration(difference, { round: true })}
               </Tooltip>;
    }
    return <Tooltip {...props}>
             {humanizeDuration(difference, { round: true })} ago
           </Tooltip>;
};

class VeloTimestamp extends Component {
    static propTypes = {
        usec: PropTypes.any,
        iso: PropTypes.any,
    }

    render() {
        var value = this.props.iso || this.props.usec;

        var ts;
        // Maybe the timestamp is specified as an iso
        if (_.isString(value)) {
            let parsed = new Date(value);
            if (!_.isNaN(parsed.getTime())) {
                ts = parsed;
            }
        }

        // If the timestamp is a number then it might be in usec
        if (!ts && !_.isNaN(value) &&  _.isNumber(value) && value > 0) {
            // Or maybe in seconds since epoch.
            if (value > 20000000000) {
                value /= 1000;
            }

            if (value > 20000000000) {
                value /= 1000;
            }

            let parsed = new Date(value * 1000);
            if (!_.isNaN(parsed.getTime())) {
                ts = parsed;
            };
        }

        // Could not parse it - just return what we got.
        if (!ts) {
            return <div>{value && JSON.stringify(value)}</div>;
        }

        var when = moment(ts);
        return <OverlayTrigger
                 delay={{show: 250, hide: 400}}
                 overlay={(props)=>renderToolTip(props, ts)}>
                 <div className="timestamp">
                   {when.utc().format('YYYY-MM-DD HH:mm:ss') + ' UTC'}
                 </div>
               </OverlayTrigger>;
    };
}

export default VeloTimestamp;
