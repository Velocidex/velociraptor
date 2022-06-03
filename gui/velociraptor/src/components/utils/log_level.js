import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.js';
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
        case "DEBUG":
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
