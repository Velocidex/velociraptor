import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';
import './log-level.css';

export default class LogLevel extends Component {
    static propTypes = {
        type: PropTypes.string,
    };

    render() {
        let icon;
        switch (this.props.type) {
        case "ERROR":
        case "INFO":
        case "WARN":
        case "DEBUG":
        case "DEFAULT":
        case "ALERT":
            icon = T(this.props.type);
            break;
        default:
            icon = <></>;
        }

        return <Button variant="outline-default"
                       className={'log-level level-' + this.props.type}>
                 {icon}
               </Button>;
    }
}
