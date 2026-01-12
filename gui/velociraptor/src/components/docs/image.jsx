import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';

class ImageDialog extends Component {
    static propTypes = {
        alt: PropTypes.string,
        src: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    }

    render() {
        return (
            <Modal show={true}
                   className="image-popup"
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{this.props.alt}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <img className="fullscreen-popup"
                  alt={this.props.alt} src={this.props.src} />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class Image extends Component {
    static propTypes = {
        alt: PropTypes.string,
        src: PropTypes.string,
    }

    state = {
        expanded: false,
    }

    render() {
        return <>
                 <a className="image-popup"
                    onClick={()=>this.setState({expanded: true})}>
                   <span><img alt={this.props.alt} src={this.props.src} /></span>
                   <span className="figcaption">{this.props.alt}</span>
                 </a>
                 {this.state.expanded &&
                  <ImageDialog
                    onClose={()=>this.setState({expanded: false})}
                    alt={this.props.alt}
                    src={this.props.src} />}
               </>;
    }
}
