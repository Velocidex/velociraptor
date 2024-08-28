import "./url.css";

import React, { Component } from 'react';

import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import api from '../core/api-service.jsx';


export default class URLViewer extends Component {
    static propTypes = {
        // The URL can be formatted as a markdown URL: [text](url)
        // Alternativly an explicit description can be passed to the link.
        url: PropTypes.string,
        desc: PropTypes.string,
        safe: PropTypes.bool,

        // If specified this is an intenral link - so we add org id
        // etc.
        internal: PropTypes.bool,
    }

    state = {
        show: false,
    }

    render() {
        let url = this.props.url || "";
        let desc = url;
        const md_regex = url.match(/\[([^\]]+)\]\(([^)]+)\)/);
        if (md_regex) {
            url = md_regex[2];
            desc = md_regex[1];
        }

        if(this.props.desc) {
            desc = this.props.desc;
        }

        if (!this.props.safe) {
            return <Button
                     as="a"
                     className="url-link"
                     size="sm"
                     variant="outline-info"
                     href={api.href(url, {}, {
                         internal: this.props.internal,
                     })} target="_blank">
                     <FontAwesomeIcon icon="external-link-alt"/>
                     <span className="button-label">{desc}</span>
                   </Button>;
        };

        return <>
                 <Button
                   className="url-link"
                   size="sm"
                   variant="outline-info"
                   onClick={()=>this.setState({show: true})}>
                   <FontAwesomeIcon icon="external-link-alt"/>
                   <span className="button-label">{desc}</span>
                 </Button>

                 <Modal show={this.state.show}
                        dialogClassName="modal-90w"
                        onHide={(e) => this.setState({show: false})}>
                   <Modal.Header closeButton>
                     <Modal.Title>Raw Response JSON</Modal.Title>
                   </Modal.Header>

                   <Modal.Body>
                     <h3>Confirm link open</h3>

                     <Button
                       as="a"
                       className="url-link"
                       size="lg"
                       variant="outline-info"
                       href={url} target="_blank">
                       <FontAwesomeIcon icon="external-link-alt"/>
                       <span className="button-label">{desc} ({url})</span>
                     </Button>
                   </Modal.Body>

                   <Modal.Footer>
                     <Navbar className="w-100 justify-content-between">
                       <ButtonGroup className="float-right">
                         <Button variant="secondary"
                                 onClick={() => this.setState({show: false})} >
                           Close
                         </Button>
                       </ButtonGroup>
                     </Navbar>
                   </Modal.Footer>
                 </Modal>
               </>;
    }
}
