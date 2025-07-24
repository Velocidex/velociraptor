import "./time.css";
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment';
import 'moment-timezone';
import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import ContextMenu from './context.jsx';

const renderHumanTime = ts=> {
    if (!_.isDate(ts)) {
        return "";
    }

    let now = new Date().getTime();
    let difference = (now-ts.getTime());
    return T("HumanizeDuration", difference);
};

const digitsRegex = /^[0-9.]+$/;
//
// The log contains timestamps like: '2022-08-19 23:34:53.165297306 +0000 UTC'
// Date() can only handle '2022-08-19 23:34:53.165+00:00'
const timestampWithNanoRegex = /(\d{4}-\d{2}-\d{2} \d{1,2}:\d{2}:\d{2}\.\d{3})\d+ ([-+]\d{2})(\d{2})/;

// Try hard to convert the value into a proper timestamp.
export const ToStandardTime = value => {
    // Maybe the timestamp is specified as an iso string
    if (_.isString(value)) {
        // Really an int that got converted to a string along the way
        // (can happen with 64 bit ints which JSON does not support).
        if (digitsRegex.test(value)) {
            value = parseFloat(value);

        } else {

            // Maybe an iso string
            let parsed = new Date(value);
            if (!_.isNaN(parsed.getTime())) {

                // Ok this is fine.
                return parsed;
            };

            const m = value.match(timestampWithNanoRegex);
            if (m) {
                let newDate = m[1] + m[2] + ":" + m[3];

                parsed = new Date(newDate);
                if (!_.isNaN(parsed.getTime())) {
                    return parsed;
                }
            }
        }
    }

    // If the timestamp is anumber then it might be in sec, msec,
    // usec or nsec - we want to support all of those.
    if (!_.isNaN(value) &&  _.isNumber(value) && value > 0) {
        // Maybe nsec
        if (value > 20000000000000000) {  // 11 October 2603 in microsec
            value /= 1000000;  // To msec

        } else if (value > 20000000000000) {  // 11 October 2603 in milliseconds
            value /= 1000;

        } else if (value > 20000000000) { // 11 October 2603 in seconds
            // Already in msec.

        } else {
            // Might be in sec - convert to ms.
            value = value * 1000;
        }


        let parsed = new Date(value);
        if (!_.isNaN(parsed.getTime())) {
            return parsed;
        };
    }
    return value;
};

export const FormatRFC3339 = (ts, timezone) => {
    let when = moment(ts);
    let when_tz = moment.tz(when, timezone);
    let formatted_ts = when_tz.format("YYYY-MM-DDTHH:mm:ss");

    let fractional_part = when_tz.format(".SSS");
    if (fractional_part === ".000") {
        fractional_part = "";
    }
    formatted_ts += fractional_part;
    if (when_tz.isUtc()) {
        formatted_ts += "Z";
    } else {
        formatted_ts += when_tz.format("Z");
    }

    return formatted_ts;
};

class VeloTimestamp extends Component {
    static contextType = UserConfig;

    static propTypes = {
        usec: PropTypes.any,
        iso: PropTypes.any,
    }

    render() {
        let value = this.props.iso || this.props.usec;
        let ts = ToStandardTime(value);
        if (_.isNaN(ts)) {
            return <></>;
        }

        // Could not parse it - just return what we got.
        if (!ts) {
            return <div>{value && JSON.stringify(value)}</div>;
        }

        let timezone = this.context.traits.timezone || "UTC";
        let formatted_ts = FormatRFC3339(ts, timezone);
        if (formatted_ts.match("Invalid date")) {
            return <></>;
        }

        return <>
                 <ContextMenu value={formatted_ts}>
                   <ToolTip tooltip={renderHumanTime(ts)}>
                     <div className="timestamp">
                       {formatted_ts}
                     </div>
                   </ToolTip>
                 </ContextMenu>
               </>;
    };
}

export default VeloTimestamp;

// Returns a date object in local timestamp which represents the UTC
// date. This is needed because the date selector widget expects to
// work in local time.
export function localTimeFromUTCTime(date) {
    let msSinceEpoch = date.getTime();
    let tzoffset = (new Date()).getTimezoneOffset();
    return new Date(msSinceEpoch + tzoffset * 60000);
}

export function utcTimeFromLocalTime(date) {
    let msSinceEpoch = date.getTime();
    let tzoffset = (new Date()).getTimezoneOffset();
    return new Date(msSinceEpoch - tzoffset * 60000);
}
