import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import PropTypes from 'prop-types';
import { Link } from "react-router-dom";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class VeloButton extends Component {
    static propTypes = {
        href: PropTypes.string,
        text: PropTypes.string,
        icon: PropTypes.string,
    }

    render() {
        return (
            <a href={this.props.href}

               role="button" className="btn btn-default">
              {this.props.icon &&
               <span className="icon-small">
                 <FontAwesomeIcon icon={this.props.icon} />
               </span>}
              { this.props.text }
            </a>
        );
    }
}
